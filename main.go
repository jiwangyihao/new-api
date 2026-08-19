package main

import (
	"bytes"
	"embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	_ "github.com/QuantumNous/new-api/setting/performance_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	_ "net/http/pprof"
)

func serverListenAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	if host := os.Getenv("HOST"); host != "" {
		return host + ":" + port
	}
	return ":" + port
}

func pprofListenAddr() string {
	if addr := os.Getenv("PPROF_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:8005"
}

func pprofMonitorEnabled() bool {
	return os.Getenv("ENABLE_PPROF_MONITOR") == "true"
}
func loadDotEnv() {
	if err := godotenv.Load(".env"); err != nil && common.DebugEnabled {
		common.SysLog("No .env file found, using default environment variables. If needed, please create a .env file and set the relevant variables.")
	}
}

type runtimeStartupPlan struct {
	maintenanceMode    bool
	startApplication   bool
	startRedis         bool
	startBackground    bool
	startSystemMonitor bool
	startHTTP          bool
}

func maintenanceModeEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MAINTENANCE_MODE")), "true")
}

func runtimeStartupPlanFor(maintenanceMode bool) runtimeStartupPlan {
	return runtimeStartupPlan{
		maintenanceMode:    maintenanceMode,
		startApplication:   !maintenanceMode,
		startRedis:         !maintenanceMode,
		startBackground:    !maintenanceMode,
		startSystemMonitor: !maintenanceMode,
		startHTTP:          !maintenanceMode,
	}
}
func waitForMaintenanceShutdown(signals <-chan os.Signal) {
	<-signals
}

const maintenanceReadinessFile = "/tmp/new-api-maintenance-ready"
const maintenanceSchemaReadinessFile = "/tmp/new-api-credit-valuation-schema-ready"

func writeMaintenanceReadiness(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	_, err = file.WriteString("ready\n")
	return err
}

func runMaintenanceSession(path string, signals <-chan os.Signal) error {
	if err := writeMaintenanceReadiness(path); err != nil {
		return fmt.Errorf("failed to write maintenance readiness: %w", err)
	}
	defer func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			common.SysError("failed to remove maintenance readiness: " + err.Error())
		}
	}()
	waitForMaintenanceShutdown(signals)
	return nil
}

func runMaintenanceModeSession(readinessPath string, schemaReadinessPath string, signals <-chan os.Signal) error {
	defer func() {
		if err := os.Remove(schemaReadinessPath); err != nil && !os.IsNotExist(err) {
			common.SysError("failed to remove maintenance schema readiness: " + err.Error())
		}
	}()
	return runMaintenanceSession(readinessPath, signals)
}

func blockInMaintenanceMode() error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	common.SysLog("maintenance mode enabled; skipped Redis, background tasks, system monitor, profiling, and HTTP server; waiting for termination signal")
	return runMaintenanceModeSession(maintenanceReadinessFile, maintenanceSchemaReadinessFile, signals)
}

func stageMaintenanceSchema(readinessPath string, initializeAndMigrate func() error, closeDatabase func() error) error {
	if err := os.Remove(readinessPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear maintenance schema readiness: %w", err)
	}
	if err := initializeAndMigrate(); err != nil {
		if closeErr := closeDatabase(); closeErr != nil {
			return fmt.Errorf("failed to initialize maintenance schema: %w; failed to close maintenance database: %v", err, closeErr)
		}
		return fmt.Errorf("failed to initialize maintenance schema: %w", err)
	}
	if err := writeMaintenanceReadiness(readinessPath); err != nil {
		if closeErr := closeDatabase(); closeErr != nil {
			return fmt.Errorf("failed to publish maintenance schema readiness: %w; failed to close maintenance database: %v", err, closeErr)
		}
		return fmt.Errorf("failed to publish maintenance schema readiness: %w", err)
	}
	return nil
}

func closeRuntimeDatabases(maintenanceMode bool) error {
	if maintenanceMode {
		return model.CloseMaintenanceDB()
	}
	return model.CloseDB()
}

func channelUpdateFrequencyFromEnv(value string) (int, bool, error) {
	if value == "" {
		return 0, false, nil
	}
	frequency, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, err
	}
	if frequency <= 0 {
		return 0, false, nil
	}
	return frequency, true, nil
}

//go:embed web/default/dist
var buildFS embed.FS

//go:embed web/default/dist/index.html
var indexPage []byte

//go:embed web/classic/dist
var classicBuildFS embed.FS

//go:embed web/classic/dist/index.html
var classicIndexPage []byte

func main() {
	if len(os.Args) > 1 && os.Args[1] == "credit-valuation-migrate" {
		os.Exit(RunCreditValuationCommand(os.Args[2:], os.Stdout, os.Stderr))
	}
	startTime := time.Now()
	loadDotEnv()
	startupPlan := runtimeStartupPlanFor(maintenanceModeEnabled())

	err := InitResources()
	if err != nil {
		common.FatalLog("failed to initialize resources: " + err.Error())
		return
	}

	defer func() {
		if err := closeRuntimeDatabases(startupPlan.maintenanceMode); err != nil {
			common.FatalLog("failed to close database: " + err.Error())
		}
	}()

	if !startupPlan.startApplication {
		if err := blockInMaintenanceMode(); err != nil {
			common.FatalLog(err.Error())
		}
		return
	}

	common.SysLog("New API " + common.Version + " started")
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
	}

	if common.RedisEnabled {
		// for compatibility with old versions
		common.MemoryCacheEnabled = true
	}
	if common.MemoryCacheEnabled {
		common.SysLog("memory cache enabled")
		common.SysLog(fmt.Sprintf("sync frequency: %d seconds", common.SyncFrequency))

		// Add panic recovery and retry for InitChannelCache
		func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog(fmt.Sprintf("InitChannelCache panic: %v, retrying once", r))
					// Retry once
					_, _, fixErr := model.FixAbility()
					if fixErr != nil {
						common.FatalLog(fmt.Sprintf("InitChannelCache failed: %s", fixErr.Error()))
					}
				}
			}()
			model.InitChannelCache()
		}()

		go model.SyncChannelCache(common.SyncFrequency)
	}

	// 热更新配置
	go model.SyncOptions(common.SyncFrequency)

	// 数据看板
	go model.UpdateQuotaData()

	frequency, channelUpdaterEnabled, err := channelUpdateFrequencyFromEnv(os.Getenv("CHANNEL_UPDATE_FREQUENCY"))
	if err != nil {
		common.FatalLog("failed to parse CHANNEL_UPDATE_FREQUENCY: " + err.Error())
	}
	if channelUpdaterEnabled {
		go controller.AutomaticallyUpdateChannels(frequency)
	}

	go controller.AutomaticallyTestChannels()

	// Codex credential auto-refresh check every 10 minutes, refresh when expires within 1 day
	service.StartCodexCredentialAutoRefreshTask()

	// Subscription quota reset task (daily/weekly/monthly/custom)
	service.StartSubscriptionQuotaResetTask()

	// Invitation entitlement refresh task (daily Asia/Shanghai midnight cache refresh)
	service.StartInvitationEntitlementRefreshTask()

	// Invitation reward event retry task (compensates failed post-payment dispatch)
	service.StartInvitationRewardEventRetryTask()

	// Wire task polling adaptor factory (breaks service -> relay import cycle)
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		a := relay.GetTaskAdaptor(platform)
		if a == nil {
			return nil
		}
		return a
	}

	// Channel upstream model update check task
	controller.StartChannelUpstreamModelUpdateTask()
	// Kyren pending subscription reconciliation task
	controller.StartKyrenReconciliationTask()

	if common.IsMasterNode && constant.UpdateTask {
		gopool.Go(func() {
			controller.UpdateMidjourneyTaskBulk()
		})
		gopool.Go(func() {
			controller.UpdateTaskBulk()
		})
	}
	if os.Getenv("BATCH_UPDATE_ENABLED") == "true" {
		common.BatchUpdateEnabled = true
		common.SysLog("batch update enabled with interval " + strconv.Itoa(common.BatchUpdateInterval) + "s")
		model.InitBatchUpdater()
	}

	if os.Getenv("ENABLE_PPROF") == "true" {
		gopool.Go(func() {
			log.Println(http.ListenAndServe(pprofListenAddr(), nil))
		})
		common.SysLog("pprof enabled")
	}
	if pprofMonitorEnabled() {
		go common.Monitor()
		common.SysLog("automatic pprof monitor enabled")
	}

	err = common.StartPyroScope()
	if err != nil {
		common.SysError(fmt.Sprintf("start pyroscope error : %v", err))
	}

	// Initialize HTTP server
	server := gin.New()
	server.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		common.SysLog(fmt.Sprintf("panic detected: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Panic detected, error: %v. Please submit a issue here: https://github.com/Calcium-Ion/new-api", err),
				"type":    "new_api_panic",
			},
		})
	}))
	// This will cause SSE not to work!!!
	//server.Use(gzip.Gzip(gzip.DefaultCompression))
	server.Use(middleware.RequestId())
	server.Use(middleware.PoweredBy())
	server.Use(middleware.I18n())
	middleware.SetUpLogger(server)
	// Initialize session store
	store := cookie.NewStore([]byte(common.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000, // 30 days
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
	server.Use(sessions.Sessions("session", store))

	InjectUmamiAnalytics()
	InjectGoogleAnalytics()

	listenAddr := serverListenAddr()
	loadtestHTTPStats := controller.NewLoadtestHTTPStats()
	// 设置路由
	router.SetRouter(server, router.ThemeAssets{
		DefaultBuildFS:   buildFS,
		DefaultIndexPage: indexPage,
		ClassicBuildFS:   classicBuildFS,
		ClassicIndexPage: classicIndexPage,
	}, listenAddr, loadtestHTTPStats)
	var port = os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}

	// Log startup success message
	common.LogStartupSuccess(startTime, port)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		common.FatalLog("failed to start HTTP server: " + err.Error())
	}
	defer listener.Close()

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: server.Handler(),
		ConnState: func(_ net.Conn, state http.ConnState) {
			loadtestHTTPStats.OnConnState(state)
		},
	}
	if gin.IsDebugging() {
		common.LogWriterMu.RLock()
		fmt.Fprintf(gin.DefaultWriter, "[GIN-debug] Listening and serving HTTP on %s\n", listenAddr)
		common.LogWriterMu.RUnlock()
	}
	err = httpServer.Serve(controller.NewLoadtestCountingListener(listener, loadtestHTTPStats))
	if err != nil {
		common.FatalLog("failed to start HTTP server: " + err.Error())
	}
}

func InjectUmamiAnalytics() {
	analyticsInjectBuilder := &strings.Builder{}
	if os.Getenv("UMAMI_WEBSITE_ID") != "" {
		umamiSiteID := os.Getenv("UMAMI_WEBSITE_ID")
		umamiScriptURL := os.Getenv("UMAMI_SCRIPT_URL")
		if umamiScriptURL == "" {
			umamiScriptURL = "https://analytics.umami.is/script.js"
		}
		analyticsInjectBuilder.WriteString("<script defer src=\"")
		analyticsInjectBuilder.WriteString(umamiScriptURL)
		analyticsInjectBuilder.WriteString("\" data-website-id=\"")
		analyticsInjectBuilder.WriteString(umamiSiteID)
		analyticsInjectBuilder.WriteString("\"></script>")
	}
	analyticsInjectBuilder.WriteString("<!--Umami QuantumNous-->\n")
	analyticsInject := []byte(analyticsInjectBuilder.String())
	placeholder := []byte("<!--umami-->\n")
	indexPage = bytes.ReplaceAll(indexPage, placeholder, analyticsInject)
	classicIndexPage = bytes.ReplaceAll(classicIndexPage, placeholder, analyticsInject)
}

func InjectGoogleAnalytics() {
	analyticsInjectBuilder := &strings.Builder{}
	if os.Getenv("GOOGLE_ANALYTICS_ID") != "" {
		gaID := os.Getenv("GOOGLE_ANALYTICS_ID")
		// Google Analytics 4 (gtag.js)
		analyticsInjectBuilder.WriteString("<script async src=\"https://www.googletagmanager.com/gtag/js?id=")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("\"></script>")
		analyticsInjectBuilder.WriteString("<script>")
		analyticsInjectBuilder.WriteString("window.dataLayer = window.dataLayer || [];")
		analyticsInjectBuilder.WriteString("function gtag(){dataLayer.push(arguments);}")
		analyticsInjectBuilder.WriteString("gtag('js', new Date());")
		analyticsInjectBuilder.WriteString("gtag('config', '")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("');")
		analyticsInjectBuilder.WriteString("</script>")
	}
	analyticsInjectBuilder.WriteString("<!--Google Analytics QuantumNous-->\n")
	analyticsInject := []byte(analyticsInjectBuilder.String())
	placeholder := []byte("<!--Google Analytics-->\n")
	indexPage = bytes.ReplaceAll(indexPage, placeholder, analyticsInject)
	classicIndexPage = bytes.ReplaceAll(classicIndexPage, placeholder, analyticsInject)
}

func InitResources() error {
	// Load .env before reading startup-mode flags. Keep this idempotent because
	// InitResources is also called independently by operational tooling.
	loadDotEnv()

	// 加载环境变量
	common.InitEnv()

	logger.SetupLogger()
	common.StartMemoryGuard()

	// Initialize model settings
	ratio_setting.InitRatioSettings()

	service.InitHttpClient()

	service.InitTokenEncoders()

	maintenanceMode := maintenanceModeEnabled()
	if maintenanceMode {
		// This is the explicit DDL boundary. Maintenance database initialization
		// remains connection-only so dry-run and verify commands cannot migrate.
		err := stageMaintenanceSchema(maintenanceSchemaReadinessFile, func() error {
			maintenanceDB, err := model.InitMaintenanceDB()
			if err != nil {
				return err
			}
			return model.MigrateCreditValuationSchema(maintenanceDB)
		}, model.CloseMaintenanceDB)
		if err != nil {
			return err
		}
		common.SysLog("maintenance mode enabled; maintenance schema initialized; skipped mutable initialization, Redis, cache cleanup, metrics, system monitor, and HTTP startup")
		return nil
	}

	// Initialize SQL Database and run the normal startup migrations.
	err := model.InitDB()
	if err != nil {
		common.FatalLog("failed to initialize database: " + err.Error())
		return err
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DISTRIBUTOR_DEFAULT_PLANS_ENABLED")), "true") {
		if err := model.EnsureDistributorDefaultPlans(); err != nil {
			common.SysError("failed to ensure distributor default subscription plans: " + err.Error())
		}
	}

	model.CheckSetup()

	// Initialize options, should after model.InitDB()
	model.InitOptionMap()

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		return err
	}
	if err = model.EnsureAccountBalanceCentsMigration(); err != nil {
		common.FatalLog("failed to migrate account balance cents: " + err.Error())
		return err
	}

	// 清理旧的磁盘缓存文件
	common.CleanupOldCacheFiles()

	// 初始化模型
	model.GetPricing()

	// Initialize SQL Database
	err = model.InitLogDB()
	if err != nil {
		return err
	}

	perfmetrics.Init()

	// 启动系统监控
	common.StartSystemMonitor()

	// Initialize i18n
	err = i18n.Init()
	if err != nil {
		common.SysError("failed to initialize i18n: " + err.Error())
	} else {
		common.SysLog("i18n initialized with languages: " + strings.Join(i18n.SupportedLanguages(), ", "))
	}
	i18n.SetUserLangLoader(model.GetUserLanguage)

	// Load custom OAuth providers from database
	err = oauth.LoadCustomProviders()
	if err != nil {
		common.SysError("failed to load custom OAuth providers: " + err.Error())
	}

	return nil
}
