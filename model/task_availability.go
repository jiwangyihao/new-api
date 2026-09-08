package model

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
)

// Task availability is published after the task transaction commits. A durable
// pending marker enables bounded replay after a separate log database outage.
func publishTaskAvailability(task *Task) {
	if !task.AvailabilityPending || (task.Status != TaskStatusSuccess && task.Status != TaskStatusFailure) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := publishAsyncAvailability(ctx, task.AvailabilityData, string(task.Status), task.FinishTime); err != nil {
		logger.LogError(ctx, "task availability outcome not recorded: "+err.Error())
		return
	}
	if err := DB.WithContext(ctx).Model(&Task{}).Where("id = ?", task.ID).Update("availability_pending", false).Error; err != nil {
		logger.LogError(ctx, "task availability publication marker not saved: "+err.Error())
	}
}

func publishMidjourneyAvailability(task *Midjourney) {
	if !task.AvailabilityPending || (task.Status != "SUCCESS" && task.Status != "FAILURE") {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	finished := task.FinishTime
	if finished > 100000000000 {
		finished /= 1000
	}
	if err := publishAsyncAvailability(ctx, task.AvailabilityData, task.Status, finished); err != nil {
		logger.LogError(ctx, "drawing availability outcome not recorded: "+err.Error())
		return
	}
	if err := DB.WithContext(ctx).Model(&Midjourney{}).Where("id = ?", task.Id).Update("availability_pending", false).Error; err != nil {
		logger.LogError(ctx, "drawing availability publication marker not saved: "+err.Error())
	}
}

func publishAsyncAvailability(ctx context.Context, data, status string, finished int64) error {
	var observation AvailabilityObservation
	if err := common.UnmarshalJsonStr(data, &observation); err != nil {
		return err
	}
	if finished <= 0 {
		finished = time.Now().Unix()
	}
	observation.CompletedAt = finished
	observation.Outcome = AvailabilityFailure
	observation.Reason = "task_failed"
	if status == "SUCCESS" {
		observation.Outcome = AvailabilitySuccess
		observation.Reason = "task_completed"
	}
	if observation.GroupID == 0 {
		observation.Outcome = AvailabilityUnknown
		observation.Reason = "attribution_unverified"
	}
	if err := RecordAvailability(ctx, observation); err != nil {
		MarkAvailabilityGap(finished, finished+1)
		return err
	}
	return nil
}

// ReplayTaskAvailability retries committed terminal tasks without polling any
// provider. A maximum of 100 rows per kind bounds maintenance work.
func ReplayTaskAvailability(ctx context.Context) error {
	var tasks []Task
	if err := DB.WithContext(ctx).Where("availability_pending = ? AND status IN ?", true, []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Order("id").Limit(100).Find(&tasks).Error; err != nil {
		return err
	}
	for i := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		publishTaskAvailability(&tasks[i])
	}
	var drawings []Midjourney
	if err := DB.WithContext(ctx).Where("availability_pending = ? AND status IN ?", true, []string{"SUCCESS", "FAILURE"}).Order("id").Limit(100).Find(&drawings).Error; err != nil {
		return err
	}
	for i := range drawings {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		publishMidjourneyAvailability(&drawings[i])
	}
	return nil
}
