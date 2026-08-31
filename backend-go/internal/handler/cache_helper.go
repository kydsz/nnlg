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
)

// statsCacheTTL 统计报表缓存时长。评教数据非实时，可接受短延迟。
const statsCacheTTL = 60 * time.Second

// cacheKey 生成统计缓存键：接口名 + 用户ID + 查询参数哈希。
// 用户ID 必须参与，因为统计结果按数据权限（学院/校区范围）过滤，不同用户看到不同数据。
func cacheKey(api string, c *gin.Context) string {
	uid := 0
	if u := middleware.CurrentUser(c); u != nil {
		uid = u.ID
	}
	h := md5.Sum([]byte(c.Request.URL.RawQuery))
	return cache.GetClient().KeyPrefix() + "stats:" + api + ":" + strconv.Itoa(uid) + ":" + hex.EncodeToString(h[:])
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
