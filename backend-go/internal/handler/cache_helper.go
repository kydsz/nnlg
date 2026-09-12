package handler

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"time"

	"backend-go/internal/cache"
	"backend-go/internal/middleware"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// statsCacheTTL 统计报表缓存时长。评教数据非实时，可接受短延迟。
const statsCacheTTL = 60 * time.Second

// cacheKey 生成统计缓存键：接口名 + 用户ID + 缓存代际 + 查询参数哈希。
// 用户ID 必须参与，因为统计结果按数据权限（学院/校区范围）过滤，不同用户看到不同数据；
// 代际号让「权限/学院变更后立即失效」成为 O(1) 的自增，而不是按前缀扫描删除。
func cacheKey(api string, c *gin.Context) string {
	uid := 0
	if u := middleware.CurrentUser(c); u != nil {
		uid = u.ID
	}
	globalGen, userGen := cache.StatsGenerations(c.Request.Context(), uid)
	h := md5.Sum([]byte(c.Request.URL.RawQuery))
	return cache.GetClient().KeyPrefix() + "stats:" + api + ":" + strconv.Itoa(uid) +
		":" + strconv.FormatInt(globalGen, 10) + ":" + strconv.FormatInt(userGen, 10) +
		":" + hex.EncodeToString(h[:])
}

// invalidateUserAuth 主动失效用户快照 + 统计缓存
// （用户基础信息/角色/学院/教研室变更后调用：统计结果口径依赖这些字段）。
func invalidateUserAuth(c *gin.Context, uid int) {
	cache.InvalidateUserAuth(c.Request.Context(), uid)
	cache.InvalidateStats(c.Request.Context(), uid)
}

// invalidateAllStats 全局失效统计缓存（学院/校区/维度等影响所有查看者的变更后调用）。
func invalidateAllStats(c *gin.Context) { cache.InvalidateAllStats(c.Request.Context()) }

// invalidateUserSession 撤销用户全部会话（登出/禁用/改密/删除），并失效快照。
func invalidateUserSession(c *gin.Context, db *gorm.DB, uid int) {
	cache.IncrSessionEpoch(c.Request.Context(), db, uid)
}

// invalidateDimensionCache 主动失效启用维度缓存 + 全局统计缓存（维度增删改/排序后调用：
// 维度决定评分口径，统计结果必须立即失效）。
func invalidateDimensionCache(c *gin.Context) {
	cache.InvalidateDimensionCache(c.Request.Context())
	cache.InvalidateAllStats(c.Request.Context())
}

// cachedJSON 统一缓存包装：命中缓存直接返回并写响应；未命中调用 gen 生成 data 后回写缓存。
// gen 返回的 data 会作为响应 Body.Data 输出。Redis 未启用时自动降级直查。
func cachedJSON(c *gin.Context, api string, gen func() (interface{}, error)) {
	rdb := cache.GetClient()
	if rdb == nil || !rdb.Enabled {
		// 降级：直接查库
		data, err := gen()
		if err != nil {
			serverErr(c, "查询失败")
			return
		}
		responseOK(c, data)
		return
	}
	key := cacheKey(api, c)
	var data interface{}
	if rdb.GetJSON(c.Request.Context(), key, &data) {
		responseOK(c, data)
		return
	}
	data, err := gen()
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	rdb.SetJSON(c.Request.Context(), key, data, statsCacheTTL)
	responseOK(c, data)
}

// responseOK 输出 {code,message,data}，对齐 response.OK 的字段
func responseOK(c *gin.Context, data interface{}) {
	response.OK(c, data)
}
