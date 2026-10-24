package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/handler"
	"hyjs-server/httpClient"
	"hyjs-server/middleware"
	"hyjs-server/routes"
	"hyjs-server/service"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// main 初始化游戏服配置和存档，再同时开放 HTTP 与 TCP。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var (
		env           = flag.String("env", "dev", "config env: dev|test|prd")
		bind          = flag.String("bind", "", "override listen address")
		advertiseHost = flag.String("advertise-host", "", "address reachable by the client")
		tables        = flag.String("tables", "", "override APK tables directory")
		serverID      = flag.Int64("server-id", 0, "trusted deployment server ID; 0 uses game.serverId")
		database      = flag.String("db", "", "override SQLite database")
		httpPort      = flag.Int("http-port", -1, "override HTTP port, 0 selects a free port")
		tcpPort       = flag.Int("tcp-port", -1, "override TCP port, 0 selects a free port")
		logFile       = flag.String("log-file", "", "isolated process log path; empty keeps configured path")
		testPayments  = flag.Bool("enable-test-payments", false, "enable loopback-only demo payment confirmation")
		localWallet   = flag.Bool("enable-local-wallet", false, "enable local demo wallet payments in dev/test; no real charge")
	)
	flag.Parse()
	if err := config.Init(*env); err != nil {
		return err
	}
	cfg := config.AppConfigIns.Game
	autoDemo := *localWallet || cfg.EnableLocalWallet
	if autoDemo && (!(*testPayments || cfg.EnableTestPayments) || (*env != "dev" && *env != "test")) {
		return errors.New("蓝绿终端仅允许dev/test并显式启用测试支付")
	}
	if *serverID != 0 {
		cfg.ServerID = *serverID
	}
	if cfg.ServerID < 1 || cfg.ServerID > 2147483647 {
		return errors.New("游戏区服编号必须为正int32部署配置")
	}
	if *bind != "" {
		cfg.Bind = *bind
	}
	if *advertiseHost != "" {
		cfg.AdvertiseHost = *advertiseHost
	}
	if *tables != "" {
		cfg.Tables = *tables
	}
	if *database != "" {
		cfg.Database = *database
		cfg.Driver = "sqlite"
	}
	if *httpPort >= 0 {
		cfg.HTTPPort = *httpPort
	}
	if *tcpPort >= 0 {
		cfg.TCPPort = *tcpPort
	}
	if cfg.Bind == "" || cfg.AdvertiseHost == "" || (cfg.Driver == "sqlite" && cfg.Database == "") || cfg.Tables == "" {
		return errors.New("Incomplete game configuration")
	}
	if *logFile != "" {
		config.AppConfigIns.LogConfig.LogFile = *logFile
	}
	elog.LogInit()
	defer elog.Sync()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// 显式 --db 保留隔离 SQLite 测试，正式存档默认进入 MySQL。
	switch cfg.Driver {
	case "mysql":
		mc := config.AppConfigIns.MysqlConfig
		if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: mc.Host, User: mc.User, Password: mc.Pass, Database: mc.DbName}); err != nil {
			return err
		}
	case "sqlite":
		if err := db.InitGame(ctx, cfg.Database); err != nil {
			return err
		}
	default:
		return errors.New("Unsupported game database driver")
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	s := &service.GameService{Host: cfg.AdvertiseHost, ServerID: cfg.ServerID}
	s.P3AutoConfirmDemoPayments = autoDemo
	s.P3LocalWalletEnabled = autoDemo
	s.LocalPlayToolsEnabled = autoDemo
	if err = s.LoadTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InstallGGOItems(); err != nil {
		return err
	}
	// 全服日历独立于钱包；启动ctx也提供给后台业务定时器。
	if err = s.InitializeServerGameClock(ctx); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	// Global identity is migrated before per-role calendars and account-owned wallets.
	if err = s.InitializeAdminGameServers(ctx); err != nil {
		return err
	}
	if err = s.InitializeGlobalGameAccounts(ctx); err != nil {
		return err
	}
	// 六绿采用同源兼容表与原动作，原始制品及既有培养数值保留。
	if err = s.ApplyP15GreenHeroCompatibility(); err != nil {
		return err
	}
	if err = s.ApplyThreeOrangeSkillFactors(); err != nil {
		return err
	}
	if err = s.InitializeGameplayPools(ctx); err != nil {
		return err
	}
	if err = s.InitializeP2(ctx); err != nil {
		return err
	}
	// 按世界持久化玩家选择，保留原王者次数和幸运值。
	if err = s.InitializeKingRecruitWorlds(ctx); err != nil {
		return err
	}
	if err = s.InitializeP6(ctx); err != nil {
		return err
	}
	// 转盘可发原精灵实例，先准备其目录与存储，包含启动时的到期归奖。
	if err = s.LoadP9Tables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP9Pets(ctx); err != nil {
		return err
	}
	if err = s.InitializeTurntable(ctx); err != nil {
		return err
	}
	if err = s.InitializeWarriorGrowth(ctx); err != nil {
		return err
	}
	if err = s.InitializeShop(ctx); err != nil {
		return err
	}
	if err = s.InitializeTreasureBoxes(ctx); err != nil {
		return err
	}
	if err = s.InitializeActivities(ctx); err != nil {
		return err
	}
	if err = s.InitializeMonthSign(ctx); err != nil {
		return err
	}
	if err = s.SeedUserMonthSignCalendars(ctx); err != nil {
		return err
	}
	if err = s.InitializeWarriorAwake(ctx); err != nil {
		return err
	}
	if err = s.InitEquipmentMutations(ctx); err != nil {
		return err
	}
	if err = s.InitializeOpenServerActivity(ctx); err != nil {
		return err
	}
	if err = s.InitializePlayerOpeningCalendar(ctx); err != nil {
		return err
	}
	// 日常事件及生活技能在开放协议入口前完成建表和本地规则初始化。
	if err = s.InitializeDailyTasks(ctx); err != nil {
		return err
	}
	if err = s.InitGems(ctx); err != nil {
		return err
	}
	if err = s.InitSkillSpecialty(ctx); err != nil {
		return err
	}
	if err = s.InitCooking(ctx); err != nil {
		return err
	}
	if err = s.InitializeDailyCopy(ctx); err != nil {
		return err
	}
	// 公共领奖和活动实例在开放原协议前完成建表及恢复。
	if err = s.InitializeP1(ctx); err != nil {
		return err
	}
	// 主线剧情领奖回执在开放原入口前恢复，网络重试不重复发奖。
	if err = s.InitializeStory(ctx); err != nil {
		return err
	}
	if err = s.InitializePayments(ctx); err != nil {
		return err
	}
	if err = s.InitializeFirstDouble(ctx); err != nil {
		return err
	}
	// Only initialize the default-off persistent acquisition policy.
	if err = s.InitializeGrowthRecruit(ctx); err != nil {
		return err
	}
	if err = s.InitializeLocalWallet(ctx); err != nil {
		return err
	}
	if err = s.InitializeAccountWallets(ctx); err != nil {
		return err
	}
	if err = s.InitializeLocalPlay(ctx); err != nil {
		return err
	}
	if err = s.InitializeLocalPlayActivities(ctx); err != nil {
		return err
	}
	// P4独立福利和交易目录在原协议开放前完成持久化初始化。
	if err = s.InitializeP4(ctx); err != nil {
		return err
	}
	if err = s.InitializeGGO(ctx); err != nil {
		return err
	}
	// 副本福利新域先完成迁移，默认运营开关保持关闭。
	if err = s.InitializeP4CopyBenefits(ctx); err != nil {
		return err
	}
	// 普通关卡掉落先保存具名兼容规则，战斗开始后按冻结配置结算。
	if err = s.InitializeCopyDrops(ctx); err != nil {
		return err
	}
	// 公会与组队先完成原表加载和持久化初始化，再接受成员/资产变更。
	if err = s.LoadP13GuildTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP13Guild(ctx); err != nil {
		return err
	}
	if err = s.InitializeP13GuildPrayer(ctx); err != nil {
		return err
	}
	if err = s.InitializeP13GuildTech(ctx); err != nil {
		return err
	}
	if err = s.InitializeP13Team(ctx); err != nil {
		return err
	}
	// P7仅恢复原目录及冻结计算门禁；未对照范围不开放挑战扣费和奖励。
	if err = s.LoadP7Tables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP7(ctx); err != nil {
		return err
	}
	if err = s.LoadP7ExpeditionTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP7Expedition(ctx); err != nil {
		return err
	}
	if err = s.LoadP7AwakeTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP7Awake(ctx); err != nil {
		return err
	}
	if err = s.InitializeP10Social(ctx); err != nil {
		return err
	}
	if err = s.InitializeP10FriendEnergy(ctx); err != nil {
		return err
	}
	if err = s.InitializeP10Scene(ctx); err != nil {
		return err
	}
	if err = s.InitializeP10Channel(ctx); err != nil {
		return err
	}
	if err = s.InitializeP10Lucky(ctx); err != nil {
		return err
	}
	if err = s.InitializeP10Voice(ctx); err != nil {
		return err
	}
	s.P10VoiceReady = true
	if err = s.LoadP12WorldBossTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP12WorldBoss(ctx); err != nil {
		return err
	}
	if err = s.LoadP11Tables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP11Arena(ctx); err != nil {
		return err
	}
	// 奖杯时长奖独立初始化；运营开窗与原占领来源由该域核对。
	if err = s.InitializeP11GrabRandom(ctx); err != nil {
		return err
	}
	if err = s.LoadP11ArenaDailyTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP11ArenaDaily(ctx); err != nil {
		return err
	}
	if err = s.LoadP5Tables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP5(ctx); err != nil {
		return err
	}
	if err = service.InitializeAdminGameContent(ctx); err != nil {
		return err
	}
	if err = s.InitializeReAchievements(ctx); err != nil {
		return err
	}
	if err = service.InitializeGameFeedback(ctx); err != nil {
		return err
	}
	s.P5Ready = true
	if err = s.LoadP8Tables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP8Gather(ctx); err != nil {
		return err
	}
	s.P8GatherReady = true
	if err = s.InitializeP8Life(ctx); err != nil {
		return err
	}
	s.P8LifeReady = true
	if err = s.InitializeFieldResources(ctx); err != nil {
		return err
	}
	// Install the independent default-off PK domain only after P6/P8 dependencies.
	if err = s.LoadP12FieldPVPTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP12FieldPVP(ctx); err != nil {
		return err
	}
	s.P12FieldPVPReady = true
	if err = s.InitializeP9PetNext(ctx); err != nil {
		return err
	}
	if err = s.InitializeP9PetRecycle(ctx); err != nil {
		return err
	}
	if err = s.InitializeP9CopyIntimacy(ctx); err != nil {
		return err
	}
	if err = s.InitializeP9PetIntimacy(ctx); err != nil {
		return err
	}
	s.P9PetsReady = true
	// 悬赏先冻结原表与独立事务表；Ready只标记初始化，默认政策保持关闭。
	if err = s.LoadP12WantedTables(cfg.Tables); err != nil {
		return err
	}
	if err = s.InitializeP12Wanted(ctx); err != nil {
		return err
	}
	s.P12WantedReady = true
	// 跨服互信与投递表先初始化；实际对等服/赛历由明确运营配置决定。
	httpClient.Init(nil)
	if err = s.InitializeP14Cross(ctx); err != nil {
		return err
	}
	if err = s.InitializeP14Qualification(ctx); err != nil {
		return err
	}
	if err = s.RepairLegacyFirstMission(ctx); err != nil {
		return err
	}
	// 独立目录解析原文字表和媒体，不更改战斗表指纹。
	mediaManifest := ""
	if assets := os.Getenv("HYJS_ADMIN_ASSETS"); assets != "" {
		mediaManifest = filepath.Join(assets, "media", "content-manifest.json")
	}
	if err = s.LoadAdminCatalogue(cfg.Tables, mediaManifest); err != nil {
		return err
	}
	// 后台独立账号和运营审计在开放HTTP前完成初始化。
	if err = service.AdminInitialize(ctx, os.Getenv("HYJS_ADMIN_BOOTSTRAP_FILE")); err != nil {
		return err
	}
	if err = s.InitializePlunderPermanent40(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminSproutSettings(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminGuildSettings(ctx); err != nil {
		return err
	}
	if err = service.InitializeAdminActivityModes(ctx); err != nil {
		return err
	}
	if err = service.InitializeAdminOperations(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminActivitySchedules(ctx); err != nil {
		return err
	}
	// Adopt existing live windows once after original domains are ready, before accepting players.
	if err = s.AdoptAdminActivityCalendar(ctx); err != nil {
		return err
	}
	// Re's verified meal rules are a separate editable period; older receipts remain intact.
	if err = s.InitializeReRestaurant(ctx); err != nil {
		return err
	}
	// Restore verified Re catalogue entries once and retire placeholder consumers before accepting players.
	if err = s.InitializeReActivityCatalogue(ctx); err != nil {
		return err
	}
	if err = s.InitializePersonalOpening40(ctx); err != nil {
		return err
	}
	// Apply the explicitly approved mixed-rarity balance once, preserving prior configurations in the audit.
	if err = s.InitializeRecruitBalance35(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminFields(ctx); err != nil {
		return err
	}
	if err = s.InitializeGGOFieldConfiguration(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminShopAvailability(ctx); err != nil {
		return err
	}
	if err = s.InitializeMemoryShrine55(ctx); err != nil {
		return err
	}
	if err = s.InitializeSharedFieldBoss55(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminCDK(ctx); err != nil {
		return err
	}
	// Compensation previews and delivery receipts are ready before operations requests arrive.
	if err = s.InitializeAdminBroadcast(ctx); err != nil {
		return err
	}
	if err = s.InitializeAdminMail(ctx); err != nil {
		return err
	}
	// 默认放行历史APK，更新策略仅由后台显式发布。
	if err = s.InitializeClientUpdates(ctx); err != nil {
		return err
	}
	httpListener, err := net.Listen("tcp", net.JoinHostPort(cfg.Bind, strconv.Itoa(cfg.HTTPPort)))
	if err != nil {
		return err
	}
	defer httpListener.Close()
	tcpListener, err := net.Listen("tcp", net.JoinHostPort(cfg.Bind, strconv.Itoa(cfg.TCPPort)))
	if err != nil {
		return err
	}
	defer tcpListener.Close()
	s.HTTPPort = httpListener.Addr().(*net.TCPAddr).Port
	s.TCPPort = tcpListener.Addr().(*net.TCPAddr).Port
	s.InitializeGameTableLookup()
	if err = s.InitializeGameSchema64(ctx); err != nil {
		return err
	}
	handler.ConfigureGame(s)
	handler.ConfigureTestPayments(*testPayments || cfg.EnableTestPayments)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) { c.AbortWithStatus(http.StatusInternalServerError) }))
	if err = middleware.ConfigureTrustedProxies(r, config.AppConfigIns.Security.TrustedProxies); err != nil {
		return err
	}
	r.Use(middleware.SecurityHeaders(config.AppConfigIns.Security.AdminSecureCookies))
	r.Use(handler.AdminSecurityFallback)
	// 公网入口采用精确同源策略，原无 Origin 的游戏协议保持原路径。
	r.Use(middleware.CORS())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "backend": "go", "stage": "core-systems", "database": cfg.Driver, "playable": false})
	})
	handler.RegisterAdminRoutes(r, os.Getenv("HYJS_ADMIN_ASSETS"))
	routes.RouteRegister(r)
	server := &http.Server{Handler: r, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	const serviceWorkerCount = 14
	errs := make(chan error, serviceWorkerCount)
	go func() { errs <- handler.ServeGameTCP(ctx, tcpListener, s) }()
	go func() { errs <- server.Serve(httpListener) }()
	go func() { errs <- s.ServeTurntableSettlement(ctx) }()
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p1-periods", s.ServeP1Periods) }()
	// 终端每周投放与中断批次跟随区服生命周期恢复，使用真实账务日历。
	go func() {
		errs <- service.ServeBackgroundWorker(ctx, "terminal-distributions", s.ServeTerminalDistributions)
	}()
	// 公会战自动宣战与名单冻结沿同一服务ctx运行，退出时一并收口。
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p13-union-fight", s.ServeP13UnionFight) }()
	// 同场位置与结束退场使用服务ctx，停启后恢复冻结实例和角色状态。
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p13-union-fight-field", s.ServeP13UnionFightField) }()
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p14-delivery", s.ServeP14Delivery) }()
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p10-voice", s.ServeP10Voice) }()
	go func() {
		errs <- service.ServeBackgroundWorker(ctx, "p12-pk-presentation", s.RunP12FieldPVPPresentation)
	}()
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p14-qualification", s.ServeP14Qualification) }()
	// 晋级榜版本投递随服务ctx退出，停启后恢复尚未确认的版本。
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p14-promotion-rank", s.ServeP14PromotionRank) }()
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p11-arena-daily", s.ServeP11ArenaDaily) }()
	// Limit daily ranking freezes under its original period lock and shares shutdown context.
	go func() { errs <- service.ServeBackgroundWorker(ctx, "p11-limit-daily", s.ServeP11LimitDaily) }()
	elog.Info("game server ready", zap.Int("httpPort", s.HTTPPort), zap.Int("tcpPort", s.TCPPort))
	fmt.Printf("HYJS Go ready HTTP=%s TCP=%s\n", httpListener.Addr(), tcpListener.Addr())
	remaining := serviceWorkerCount
	select {
	case <-ctx.Done():
	case err = <-errs:
		remaining--
	}
	cancel()
	// 等待正在进行的存档事务结束，再关闭数据库。
	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer shutdownCancel()
	if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
		return shutdownErr
	}
	for i := 0; i < remaining; i++ {
		select {
		case <-errs:
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
