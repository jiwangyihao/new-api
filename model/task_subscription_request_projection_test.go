package model

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const taskSubscriptionRequestProjectionIndex = "idx_tasks_subscription_request_id"

func setupTaskSubscriptionRequestProjectionTestDB(t *testing.T, ensureProjection bool) *gorm.DB {
	t.Helper()
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	if ensureProjection && !db.Migrator().HasColumn(&Task{}, "subscription_request_id") {
		require.NoError(t, db.Exec("ALTER TABLE tasks ADD COLUMN subscription_request_id VARCHAR(64) NULL").Error)
		require.NoError(t, db.Exec("CREATE INDEX "+taskSubscriptionRequestProjectionIndex+" ON tasks (subscription_request_id)").Error)
	}
	return db
}

func seedTaskProjectionCreditSubscription(t *testing.T, db *gorm.DB) (int, int) {
	t.Helper()
	user, _, _, order := seedCreditValuationOrder(t, db, PaymentProviderBalance)
	completed := completeCreditValuationOrder(t, db, &order)
	return user.Id, completed.CreditBalance.UserSubscriptionId
}

func taskProjectionValue(t *testing.T, db *gorm.DB, taskID int64) sql.NullString {
	t.Helper()
	var value sql.NullString
	require.NoError(t, db.Raw("SELECT subscription_request_id FROM tasks WHERE id = ?", taskID).Row().Scan(&value))
	return value
}

func setTaskProjectionValue(t *testing.T, task *Task, requestID string) {
	t.Helper()
	field := reflect.ValueOf(task).Elem().FieldByName("SubscriptionRequestId")
	require.True(t, field.IsValid(), "Task must expose the subscription request projection")
	value := requestID
	field.Set(reflect.ValueOf(&value))
}

func TestTaskSubscriptionRequestProjectionSchema(t *testing.T) {
	db := setupTaskSubscriptionRequestProjectionTestDB(t, false)
	require.True(t, db.Migrator().HasColumn(&Task{}, "subscription_request_id"))
	require.True(t, db.Migrator().HasIndex(&Task{}, taskSubscriptionRequestProjectionIndex))

	var columnType string
	var notNull int
	require.NoError(t, db.Raw("SELECT type, \"notnull\" FROM pragma_table_info('tasks') WHERE name = ?", "subscription_request_id").Row().Scan(&columnType, &notNull))
	require.Equal(t, "varchar(64)", strings.ToLower(columnType))
	require.Zero(t, notNull)

	var unique int
	require.NoError(t, db.Raw("SELECT `unique` FROM pragma_index_list('tasks') WHERE name = ?", taskSubscriptionRequestProjectionIndex).Row().Scan(&unique))
	require.Zero(t, unique)
}

func TestTaskInsertAndUpdateSynchronizeSubscriptionRequestProjection(t *testing.T) {
	db := setupTaskSubscriptionRequestProjectionTestDB(t, true)
	userID, subscriptionID := seedTaskProjectionCreditSubscription(t, db)

	task := &Task{
		TaskID: fmt.Sprintf("task-projection-%s", t.Name()),
		UserId: userID,
		Status: TaskStatusSubmitted,
		PrivateData: TaskPrivateData{
			BillingSource:         "subscription",
			SubscriptionId:        subscriptionID,
			SubscriptionRequestId: "request-projection-insert",
		},
	}
	require.NoError(t, task.Insert())
	inserted := taskProjectionValue(t, db, task.ID)
	require.True(t, inserted.Valid)
	require.Equal(t, task.PrivateData.SubscriptionRequestId, inserted.String)

	var reloaded Task
	require.NoError(t, db.First(&reloaded, task.ID).Error)
	reloaded.PrivateData.SubscriptionRequestId = "request-projection-update"
	setTaskProjectionValue(t, &reloaded, reloaded.PrivateData.SubscriptionRequestId)
	require.NoError(t, reloaded.Update())

	updated := taskProjectionValue(t, db, task.ID)
	require.True(t, updated.Valid)
	require.Equal(t, reloaded.PrivateData.SubscriptionRequestId, updated.String)
	var persisted Task
	require.NoError(t, db.First(&persisted, task.ID).Error)
	require.Equal(t, updated.String, persisted.PrivateData.SubscriptionRequestId)
}

func TestTaskInsertFailsClosedOnSubscriptionRequestProjectionMismatch(t *testing.T) {
	db := setupTaskSubscriptionRequestProjectionTestDB(t, true)
	projected := "request-projection-explicit"
	task := &Task{
		TaskID: fmt.Sprintf("task-projection-insert-mismatch-%s", t.Name()),
		UserId: 91_003,
		Status: TaskStatusSubmitted,
		PrivateData: TaskPrivateData{
			SubscriptionRequestId: "request-projection-json",
		},
		SubscriptionRequestId: &projected,
	}

	err := task.Insert()
	require.EqualError(t, err, "task subscription request identity projection mismatch")
	var count int64
	require.NoError(t, db.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskUpdateFailsClosedOnSubscriptionRequestProjectionMismatch(t *testing.T) {
	db := setupTaskSubscriptionRequestProjectionTestDB(t, true)
	userID, subscriptionID := seedTaskProjectionCreditSubscription(t, db)
	task := &Task{
		TaskID: fmt.Sprintf("task-projection-mismatch-%s", t.Name()),
		UserId: userID,
		Status: TaskStatusInProgress,
		PrivateData: TaskPrivateData{
			BillingSource:         "subscription",
			SubscriptionId:        subscriptionID,
			SubscriptionRequestId: "request-projection-original",
		},
	}
	require.NoError(t, task.Insert())

	var reloaded Task
	require.NoError(t, db.First(&reloaded, task.ID).Error)
	reloaded.PrivateData.SubscriptionRequestId = "request-projection-diverged"
	err := reloaded.Update()
	require.EqualError(t, err, "task subscription request identity projection mismatch")

	projection := taskProjectionValue(t, db, task.ID)
	require.True(t, projection.Valid)
	require.Equal(t, "request-projection-original", projection.String)
	var persisted Task
	require.NoError(t, db.First(&persisted, task.ID).Error)
	require.Equal(t, projection.String, persisted.PrivateData.SubscriptionRequestId)
}

func TestTaskInsertKeepsEmptySubscriptionRequestProjectionNull(t *testing.T) {
	db := setupTaskSubscriptionRequestProjectionTestDB(t, true)
	task := &Task{
		TaskID: fmt.Sprintf("task-projection-empty-%s", t.Name()),
		UserId: 91_000,
		Status: TaskStatusSubmitted,
	}
	require.NoError(t, task.Insert())
	require.False(t, taskProjectionValue(t, db, task.ID).Valid)
}

func TestTimedTaskKeepsSubscriptionRequestProjectionNull(t *testing.T) {
	db := setupTaskSubscriptionRequestProjectionTestDB(t, true)
	timed := UserSubscription{
		UserId:          91_001,
		PlanId:          91_002,
		EntitlementType: SubscriptionEntitlementTimed,
		Status:          "active",
	}
	require.NoError(t, db.Create(&timed).Error)
	task := &Task{
		TaskID: fmt.Sprintf("task-projection-timed-%s", t.Name()),
		UserId: timed.UserId,
		Status: TaskStatusSubmitted,
		PrivateData: TaskPrivateData{
			BillingSource:  "subscription",
			SubscriptionId: timed.Id,
		},
	}
	require.NoError(t, task.Insert())
	require.False(t, taskProjectionValue(t, db, task.ID).Valid)
}
