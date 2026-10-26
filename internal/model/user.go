package model

import "time"

// User 是后台管理账号。表名：user
type User struct {
	Id        int64     `xorm:"pk autoincrement" json:"id"`
	UserName  string    `xorm:"varchar(64) unique not null 'user_name'" json:"username"`
	Password  string    `xorm:"varchar(64) not null" json:"-"`
	LastLogin time.Time `json:"last_login"`
	Created   time.Time `xorm:"created" json:"created"`
	Updated   time.Time `xorm:"updated" json:"updated"`
}

// Display 返回脱敏后的账号信息。
func (u User) Display() UserView {
	return UserView{
		Id:        u.Id,
		UserName:  u.UserName,
		LastLogin: u.LastLogin,
		Created:   u.Created,
	}
}

// UserView 是可安全返回给前端的账号信息（不含口令）。
type UserView struct {
	Id        int64     `json:"id"`
	UserName  string    `json:"username"`
	LastLogin time.Time `json:"last_login"`
	Created   time.Time `json:"created"`
}
