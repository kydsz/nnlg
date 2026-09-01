package cache

import (
	"context"
	"strconv"
	"time"

	"backend-go/internal/model"
)

// authUserTTL 用户快照缓存时长。权限与数据范围不写入 access_token，
// 由该短缓存实时读取，失效延迟最小。
const authUserTTL = 60 * time.Second

// SnapshotCollege 用户快照中的学院信息
type SnapshotCollege struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	Code     string     `json:"code,omitempty"`
	CampusID *int       `json:"campus_id,omitempty"`
	Status   int16      `json:"status,omitempty"`
	JoinTime *time.Time `json:"join_time,omitempty"`
}

// SnapshotRoom 用户快照中的教研室信息
type SnapshotRoom struct {
	ID        int              `json:"id"`
	Name      string           `json:"name"`
	CollegeID int              `json:"college_id"`
	Status    int16            `json:"status,omitempty"`
	JoinTime  *time.Time       `json:"join_time,omitempty"`
	College   *SnapshotCollege `json:"college,omitempty"`
}

// AuthUserSnapshot 认证/权限中间件热路径的用户快照。
// 命中缓存即可重建 *model.User，避免每次请求回源多张表。
type AuthUserSnapshot struct {
	ID                 int               `json:"id"`
	UserNo             string            `json:"user_no"`
	Username           string            `json:"username"`
	Role               string            `json:"role"`
	Status             int16             `json:"status"`
	MustChangePassword bool              `json:"must_change_password"`
	CollegeID          *int              `json:"college_id"`
	ResearchRoomID     *int              `json:"research_room_id"`
	LastLoginTime      *time.Time        `json:"last_login_time"`
	CreateTime         *time.Time        `json:"create_time"`
	UpdateTime         *time.Time        `json:"update_time"`
	RoleCodes          []string          `json:"role_codes"`
	Permissions        []string          `json:"permissions"`
	Colleges           []SnapshotCollege `json:"colleges"`
	Rooms              []SnapshotRoom    `json:"rooms"`
	College            *SnapshotCollege  `json:"college,omitempty"`
	ResearchRoom       *SnapshotRoom     `json:"research_room,omitempty"`
}

func toLocalTime(t *time.Time) *model.LocalTime {
	if t == nil {
		return nil
	}
	lt := model.LocalTime(*t)
	return &lt
}

func toTimePtr(t *model.LocalTime) *time.Time {
	if t == nil {
		return nil
	}
	tt := t.ToTime()
	return &tt
}

// SnapshotFromUser 由完整 *model.User 生成快照。
// u.Perms 需已解析（非 nil 表示已缓存），快照保存解析后的权限。
func SnapshotFromUser(u *model.User) *AuthUserSnapshot {
	snap := &AuthUserSnapshot{
		ID: u.ID, UserNo: u.UserNo, Username: u.Username, Role: u.Role,
		Status: u.Status, MustChangePassword: u.MustChangePassword,
		CollegeID: u.CollegeID, ResearchRoomID: u.ResearchRoomID,
		LastLoginTime: toTimePtr(u.LastLoginTime),
		CreateTime:    toTimePtr(u.CreateTime), UpdateTime: toTimePtr(u.UpdateTime),
	}
	snap.RoleCodes = append([]string{}, u.RoleCodes()...)
	if u.Perms != nil {
		snap.Permissions = append([]string{}, u.Perms...)
	} else {
		snap.Permissions = []string{}
	}
	for _, uc := range u.UserColleges {
		c := SnapshotCollege{ID: uc.CollegeID, JoinTime: toTimePtr(uc.JoinTime)}
		if uc.College != nil {
			c.Name = uc.College.Name
			c.Code = uc.College.Code
			c.CampusID = uc.College.CampusID
			c.Status = uc.College.Status
		}
		snap.Colleges = append(snap.Colleges, c)
	}
	for _, ur := range u.UserRooms {
		r := SnapshotRoom{ID: ur.ResearchRoomID, JoinTime: toTimePtr(ur.JoinTime)}
		if ur.ResearchRoom != nil {
			r.Name = ur.ResearchRoom.Name
			r.CollegeID = ur.ResearchRoom.CollegeID
			r.Status = ur.ResearchRoom.Status
			if ur.ResearchRoom.College != nil {
				r.College = &SnapshotCollege{
					ID: ur.ResearchRoom.College.ID, Name: ur.ResearchRoom.College.Name,
					Code: ur.ResearchRoom.College.Code, CampusID: ur.ResearchRoom.College.CampusID,
					Status: ur.ResearchRoom.College.Status,
				}
			}
		}
		snap.Rooms = append(snap.Rooms, r)
	}
	if u.College != nil {
		snap.College = &SnapshotCollege{
			ID: u.College.ID, Name: u.College.Name, Code: u.College.Code,
			CampusID: u.College.CampusID, Status: u.College.Status,
		}
	}
	if u.ResearchRoom != nil {
		r := &SnapshotRoom{
			ID: u.ResearchRoom.ID, Name: u.ResearchRoom.Name,
			CollegeID: u.ResearchRoom.CollegeID, Status: u.ResearchRoom.Status,
		}
		if u.ResearchRoom.College != nil {
			r.College = &SnapshotCollege{
				ID: u.ResearchRoom.College.ID, Name: u.ResearchRoom.College.Name,
				Code: u.ResearchRoom.College.Code, CampusID: u.ResearchRoom.College.CampusID,
				Status: u.ResearchRoom.College.Status,
			}
		}
		snap.ResearchRoom = r
	}
	return snap
}

// ToUser 由快照重建 *model.User，填充现有下游依赖的关联与瞬态 Perms。
func (s *AuthUserSnapshot) ToUser() *model.User {
	u := &model.User{
		Model:              model.Model{ID: s.ID, CreateTime: toLocalTime(s.CreateTime), UpdateTime: toLocalTime(s.UpdateTime)},
		UserNo:             s.UserNo,
		Username:           s.Username,
		Role:               s.Role,
		CollegeID:          s.CollegeID,
		ResearchRoomID:     s.ResearchRoomID,
		Status:             s.Status,
		MustChangePassword: s.MustChangePassword,
		LastLoginTime:      toLocalTime(s.LastLoginTime),
		Perms:              append([]string{}, s.Permissions...),
	}
	for _, code := range s.RoleCodes {
		u.UserRoles = append(u.UserRoles, model.UserRole{Model: model.Model{}, UserID: s.ID, Role: code})
	}
	for _, c := range s.Colleges {
		uc := model.UserCollege{Model: model.Model{}, UserID: s.ID, CollegeID: c.ID, JoinTime: toLocalTime(c.JoinTime)}
		if c.ID != 0 || c.Name != "" {
			uc.College = &model.College{Model: model.Model{ID: c.ID}, Name: c.Name, Code: c.Code, CampusID: c.CampusID, Status: c.Status}
		}
		u.UserColleges = append(u.UserColleges, uc)
	}
	for _, r := range s.Rooms {
		rr := &model.ResearchRoom{Model: model.Model{ID: r.ID}, Name: r.Name, CollegeID: r.CollegeID, Status: r.Status}
		if r.College != nil {
			rr.College = &model.College{Model: model.Model{ID: r.College.ID}, Name: r.College.Name, Code: r.College.Code, CampusID: r.College.CampusID, Status: r.College.Status}
		}
		u.UserRooms = append(u.UserRooms, model.UserRoom{Model: model.Model{}, UserID: s.ID, ResearchRoomID: r.ID, JoinTime: toLocalTime(r.JoinTime), ResearchRoom: rr})
	}
	if s.College != nil {
		u.College = &model.College{Model: model.Model{ID: s.College.ID}, Name: s.College.Name, Code: s.College.Code, CampusID: s.College.CampusID, Status: s.College.Status}
	}
	if s.ResearchRoom != nil {
		rr := &model.ResearchRoom{Model: model.Model{ID: s.ResearchRoom.ID}, Name: s.ResearchRoom.Name, CollegeID: s.ResearchRoom.CollegeID, Status: s.ResearchRoom.Status}
		if s.ResearchRoom.College != nil {
			rr.College = &model.College{Model: model.Model{ID: s.ResearchRoom.College.ID}, Name: s.ResearchRoom.College.Name, Code: s.ResearchRoom.College.Code, CampusID: s.ResearchRoom.College.CampusID, Status: s.ResearchRoom.College.Status}
		}
		u.ResearchRoom = rr
	}
	return u
}

// AuthUserKey 用户快照缓存 key：tev:auth:user:{uid}
func (c *Client) AuthUserKey(uid int) string { return c.key("auth:user", strconv.Itoa(uid)) }

// SessionEpochKey 会话 epoch key：tev:sess:{uid}
func (c *Client) SessionEpochKey(uid int) string { return c.key("sess", strconv.Itoa(uid)) }

// GetAuthUser 读用户快照。
func (c *Client) GetAuthUser(ctx context.Context, uid int) (*AuthUserSnapshot, bool) {
	if c == nil || !c.Enabled {
		return nil, false
	}
	var snap AuthUserSnapshot
	if !c.GetJSON(ctx, c.AuthUserKey(uid), &snap) {
		return nil, false
	}
	return &snap, true
}

// SetAuthUser 写用户快照（TTL 60 秒）。
func (c *Client) SetAuthUser(ctx context.Context, uid int, snap *AuthUserSnapshot) {
	if c == nil || !c.Enabled || snap == nil {
		return
	}
	c.SetJSON(ctx, c.AuthUserKey(uid), snap, authUserTTL)
}

// DelAuthUser 主动失效用户快照。
func (c *Client) DelAuthUser(ctx context.Context, uid int) {
	if c == nil || !c.Enabled {
		return
	}
	c.Del(ctx, c.AuthUserKey(uid))
}

// IncrSessionEpoch 自增会话 epoch（撤销该用户所有 refresh/access 会话）。
func (c *Client) IncrSessionEpoch(ctx context.Context, uid int) int64 {
	if c == nil || !c.Enabled {
		return 0
	}
	n, err := c.rdb.Incr(ctx, c.SessionEpochKey(uid)).Result()
	if err != nil {
		return 0
	}
	c.rdb.Expire(ctx, c.SessionEpochKey(uid), 30*24*time.Hour)
	return n
}

// GetSessionEpoch 读取会话 epoch（key 不存在返回 0，兼容未撤销过的旧 token）。
func (c *Client) GetSessionEpoch(ctx context.Context, uid int) int64 {
	if c == nil || !c.Enabled {
		return 0
	}
	n, err := c.rdb.Get(ctx, c.SessionEpochKey(uid)).Int64()
	if err != nil {
		return 0
	}
	return n
}
