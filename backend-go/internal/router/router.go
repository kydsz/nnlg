package router

import (
	"net/http"

	"backend-go/internal/config"
	"backend-go/internal/handler"
	"backend-go/internal/middleware"
	"backend-go/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Setup(cfg *config.Config, db *gorm.DB) *gin.Engine {
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(cfg.CORSOrigins))

	// 健康检查 / 根路由
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": cfg.Version})
	})
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"name": cfg.AppName, "version": cfg.Version})
	})

	authMW := middleware.Auth(cfg.SecretKey, db)
	api := r.Group("/api/v1")

	// 认证
	authH := handler.NewAuth(cfg, db, service.NewAuth())
	auth := api.Group("/auth")
	{
		auth.POST("/login", authH.Login)
		auth.POST("/logout", authH.Logout)
		auth.GET("/me", authMW, authH.Me)
		auth.POST("/password", authMW, authH.ChangePassword)
	}

	// 用户管理
	userH := handler.NewUser(db, service.NewUser())
	users := api.Group("/users", authMW)
	{
		users.GET("", middleware.RequirePermission(db, "user:view"), userH.List)
		users.POST("/batch-status", middleware.RequirePermission(db, "user:update"), userH.BatchStatus)
		users.PUT("/me/research-room", authMW, userH.UpdateMyResearchRoom)
		users.GET("/:id", middleware.RequirePermission(db, "user:view"), userH.Get)
		users.POST("", middleware.RequirePermission(db, "user:create"), userH.Create)
		users.PUT("/:id", middleware.RequirePermission(db, "user:update"), userH.Update)
		users.DELETE("/:id", middleware.RequirePermission(db, "user:delete"), userH.Delete)
		users.PUT("/:id/status", middleware.RequirePermission(db, "user:update"), userH.UpdateStatus)
		users.POST("/:id/roles/:role", middleware.RequirePermission(db, "user:update"), userH.AddRole)
		users.DELETE("/:id/roles/:role", middleware.RequirePermission(db, "user:update"), userH.RemoveRole)
		users.POST("/:id/colleges/:college_id", middleware.RequirePermission(db, "user:update"), userH.AddCollege)
		users.DELETE("/:id/colleges/:college_id", middleware.RequirePermission(db, "user:update"), userH.RemoveCollege)
		users.POST("/:id/research-rooms/:room_id", middleware.RequirePermission(db, "user:update"), userH.AddRoom)
		users.DELETE("/:id/research-rooms/:room_id", middleware.RequirePermission(db, "user:update"), userH.RemoveRoom)
		users.GET("/:id/supervisor-scope", middleware.RequirePermission(db, "user:view"), userH.GetSupervisorScope)
		users.PUT("/:id/supervisor-scope", middleware.RequirePermission(db, "user:update"), userH.UpdateSupervisorScope)
	}

	// 角色管理
	roleH := handler.NewRoleHandler(db, service.NewRole())
	roles := api.Group("/roles", authMW)
	{
		roles.GET("/permissions", middleware.RequirePermission(db, "role:manage"), roleH.PermissionGroups)
		roles.GET("", middleware.RequirePermission(db, "user:view"), roleH.List)
		roles.GET("/:id", middleware.RequirePermission(db, "user:view"), roleH.Get)
		roles.POST("", middleware.RequirePermission(db, "role:manage"), roleH.Create)
		roles.PUT("/:id", middleware.RequirePermission(db, "role:manage"), roleH.Update)
		roles.DELETE("/:id", middleware.RequirePermission(db, "role:manage"), roleH.Delete)
	}

	// 数据同步 handler（供 /colleges/sync-from-jwxt 与 /sync、/crawl 前缀共用）
	syncH := handler.NewSync(cfg, db)

	// 组织架构：校区 / 学院 / 教研室
	orgH := handler.NewOrg(db, service.NewOrg())
	campuses := api.Group("/campuses", authMW)
	{
		campuses.GET("", middleware.RequirePermission(db, "org:view"), orgH.CampusList)
		campuses.POST("", middleware.RequirePermission(db, "campus:manage"), orgH.CampusCreate)
		campuses.GET("/:id", middleware.RequirePermission(db, "org:view"), orgH.CampusGet)
		campuses.PUT("/:id", middleware.RequirePermission(db, "campus:manage"), orgH.CampusUpdate)
		campuses.DELETE("/:id", middleware.RequirePermission(db, "campus:manage"), orgH.CampusDelete)
	}
	colleges := api.Group("/colleges", authMW)
	{
		colleges.GET("", middleware.RequirePermission(db, "org:view"), orgH.CollegeList)
		colleges.POST("", middleware.RequirePermission(db, "college:manage"), orgH.CollegeCreate)
		colleges.GET("/:id", middleware.RequirePermission(db, "org:view"), orgH.CollegeGet)
		colleges.PUT("/:id", middleware.RequirePermission(db, "college:manage"), orgH.CollegeUpdate)
		colleges.DELETE("/:id", middleware.RequirePermission(db, "college:manage"), orgH.CollegeDelete)
		colleges.POST("/sync-from-jwxt", middleware.RequirePermission(db, "role:manage"), syncH.SyncColleges)
	}
	rooms := api.Group("/research-rooms", authMW)
	{
		// 列表接口不限权限（对齐旧端），教师移动端自助设置教研室需要
		rooms.GET("", orgH.RoomList)
		rooms.POST("", middleware.RequirePermission(db, "research_room:manage"), orgH.RoomCreate)
		rooms.GET("/:id", middleware.RequirePermission(db, "org:view"), orgH.RoomGet)
		rooms.PUT("/:id", middleware.RequirePermission(db, "research_room:manage"), orgH.RoomUpdate)
		rooms.DELETE("/:id", middleware.RequirePermission(db, "research_room:manage"), orgH.RoomDelete)
	}

	// 评教维度
	dimH := handler.NewDimension(db, service.NewDimension())
	dimensions := api.Group("/dimensions", authMW)
	{
		dimensions.GET("/groups", dimH.GroupList)
		dimensions.POST("/groups", middleware.RequirePermission(db, "dimension:manage"), dimH.GroupCreate)
		dimensions.PUT("/groups/sort", middleware.RequirePermission(db, "dimension:manage"), dimH.GroupSort)
		dimensions.PUT("/groups/:id", middleware.RequirePermission(db, "dimension:manage"), dimH.GroupUpdate)
		dimensions.DELETE("/groups/:id", middleware.RequirePermission(db, "dimension:manage"), dimH.GroupDelete)
		dimensions.GET("", dimH.DimList)
		dimensions.GET("/active", dimH.DimActive)
		dimensions.PUT("/sort", middleware.RequirePermission(db, "dimension:manage"), dimH.DimSort)
		dimensions.GET("/:id", dimH.DimGet)
		dimensions.POST("", middleware.RequirePermission(db, "dimension:manage"), dimH.DimCreate)
		dimensions.PUT("/:id", middleware.RequirePermission(db, "dimension:manage"), dimH.DimUpdate)
		dimensions.DELETE("/:id", middleware.RequirePermission(db, "dimension:manage"), dimH.DimDelete)
	}

	// 评教任务
	taskH := handler.NewTask(db)
	tasks := api.Group("/tasks", authMW)
	{
		tasks.GET("", middleware.RequirePermission(db, "task:view"), taskH.List)
		tasks.GET("/:id", middleware.RequirePermission(db, "task:view"), taskH.Get)
		tasks.POST("", middleware.RequirePermission(db, "task:create"), taskH.Create)
		tasks.POST("/batch", middleware.RequirePermission(db, "task:create"), taskH.BatchCreate)
		tasks.POST("/export", middleware.RequirePermission(db, "task:view"), taskH.Export)
		tasks.PUT("/:id", middleware.RequirePermission(db, "task:update"), taskH.Update)
		tasks.DELETE("/:id", middleware.RequireAnyPermission(db, "task:delete", "task:delete_own"), taskH.Delete)
	}

	// 评教记录
	evalH := handler.NewEvaluationHandler(cfg, db)
	evaluations := api.Group("/evaluations", authMW)
	{
		evaluations.GET("", middleware.RequirePermission(db, "evaluation:view"), evalH.List)
		evaluations.POST("", middleware.RequirePermission(db, "evaluation:create"), evalH.Submit)
		evaluations.POST("/with-files", middleware.RequirePermission(db, "evaluation:create"), evalH.SubmitWithFiles)
		evaluations.GET("/:id", middleware.RequirePermission(db, "evaluation:view"), evalH.Detail)
		evaluations.GET("/:id/export", middleware.RequirePermission(db, "evaluation:view"), evalH.Export)
		evaluations.DELETE("/:id", authMW, evalH.Delete)
		evaluations.PUT("/:id", authMW, evalH.Update)
	}

	// 文件访问（兼容旧端 /files 路径）
	uploadH := handler.NewUpload(cfg, db)
	upload := api.Group("/upload", authMW)
	{
		upload.POST("", uploadH.Do)
		upload.POST("/evaluation/:task_id/:dim_code", uploadH.UploadEvaluation)
		upload.DELETE("/evaluation/:task_id/:dim_code/:filename", uploadH.DeleteEvaluationFile)
		upload.GET("/*filepath", uploadH.Serve)
	}
	api.GET("/files/*filepath", authMW, uploadH.Serve)

	// 课表查询
	schH := handler.NewSchedule(db)
	schedules := api.Group("/course-schedules", authMW, middleware.RequirePermission(db, "schedule:view"))
	{
		schedules.GET("/list", schH.List)
		schedules.GET("/detail/:id", schH.Detail)
		schedules.GET("/teacher/:teacher_id", schH.ByTeacher)
		schedules.GET("/teachers", schH.Teachers)
		schedules.GET("/teacher-scope", schH.TeacherScope)
		schedules.GET("/my-schedule", schH.MySchedule)
		schedules.GET("/versions", schH.Versions)
		schedules.GET("/teacher-status", schH.TeacherStatus)
		schedules.GET("/semesters", schH.Semesters)
		schedules.GET("/stats", schH.Stats)
		schedules.GET("/semester-configs", schH.SemesterConfigs)
		schedules.GET("/semester-configs/current", schH.CurrentSemester)
		schedules.GET("/semester-configs/:semester", schH.SemesterConfig)
		schedules.POST("/semester-configs", middleware.RequirePermission(db, "org:view"), schH.CreateSemesterConfig)
		schedules.PUT("/semester-configs/:semester", middleware.RequirePermission(db, "org:view"), schH.UpdateSemesterConfig)
		schedules.DELETE("/semester-configs/:semester", middleware.RequirePermission(db, "org:view"), schH.DeleteSemesterConfig)
	}

	// 数据同步（教务系统）
	api.POST("/teachers/sync-from-jwxt", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncTeachers)
	api.GET("/teachers/sync-status", authMW, syncH.SyncTeachersStatus)
	api.POST("/course-schedules/crawl", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.CrawlCourseSchedule)
	api.POST("/course-schedules/parse-html", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.ParseHTML)
	api.POST("/course-schedules/sync-from-jwxt", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncCourseSchedule)
	api.POST("/sync/all", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncAll)
	api.POST("/sync/course-schedule", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncCourseSchedule)
	api.POST("/sync/units-and-teachers", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncUnitsAndTeachers)
	api.POST("/sync/llsykb", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncLlsykb)
	api.POST("/sync/llsykb/preview", authMW, middleware.RequirePermission(db, "role:manage"), syncH.SyncLlsykbPreview)
	api.POST("/sync/llsykb/batch", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.SyncLlsykbBatch)
	api.GET("/sync/llsykb/progress/:taskId", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.GetLlsykbProgress)
	api.GET("/sync/scan-invalid-users", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.ScanInvalidUsers)
	api.DELETE("/sync/cleanup-user/:userId", authMW, middleware.RequirePermission(db, "sync:execute"), syncH.CleanupInvalidUser)

	// 数据爬取（对齐旧端 /crawl 前缀）
	crawl := api.Group("/crawl", authMW)
	{
		crawl.POST("/timetable", middleware.RequirePermission(db, "sync:execute"), syncH.CrawlTimetable)
		crawl.POST("/timetable/preview", middleware.RequirePermission(db, "sync:execute"), syncH.CrawlTimetablePreview)
		crawl.POST("/timetable/async", middleware.RequirePermission(db, "sync:execute"), syncH.CrawlTimetableAsync)
		crawl.POST("/llsykb", middleware.RequirePermission(db, "sync:execute"), syncH.CrawlLlsykb)
	}

	// 统计报表
	statsH := handler.NewStats(db)
	stats := api.Group("/stats", authMW, middleware.RequirePermission(db, "stats:view"))
	{
		stats.GET("/current-semester", statsH.CurrentSemester)
		stats.GET("/overview", statsH.Overview)
		stats.GET("/teachers", statsH.Teachers)
		stats.GET("/college", statsH.College)
		stats.GET("/colleges", statsH.Colleges)
		stats.GET("/campus", statsH.Campus)
		stats.GET("/campuses", statsH.Campuses)
		stats.GET("/college-teachers/:college_id", statsH.CollegeTeachers)
		stats.GET("/supervisors", statsH.Supervisors)
		stats.GET("/evaluators", statsH.Evaluators)
		stats.GET("/unteached-teachers", statsH.UnteachedTeachers)
		stats.GET("/evaluation-records", statsH.EvaluationRecords)
		stats.GET("/teacher-evaluation-summary", statsH.TeacherSummary)
		stats.GET("/export/teachers", statsH.ExportTeachers)
		stats.GET("/export/colleges", statsH.ExportColleges)
		stats.GET("/export/supervisors", statsH.ExportSupervisors)
		stats.GET("/export/teacher-evaluation-summary", statsH.ExportTeacherSummary)
		stats.POST("/evaluation-records/export", statsH.ExportEvaluationRecords)
	}

	return r
}
