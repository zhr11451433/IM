package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID       int64  `gorm:"primaryKey;autoIncrement"`
	AppKey   string `gorm:"column:app_key;size:45;not null;uniqueIndex:uniq_app_user"`
	UserID   string `gorm:"column:user_id;size:32;not null;uniqueIndex:uniq_app_user"`
	Nickname string `gorm:"column:nickname;size:50"`
	//Portrait 在这里指头像
	UserPortrait string    `gorm:"column:user_portrait;size:200"`
	UserType     int32     `gorm:"column:user_type"`
	CreatedTime  time.Time `gorm:"column:created_time;autoCreateTime:milli"`
	UpdatedTime  time.Time `gorm:"column:updated_time;autoUpdateTime:milli"`
}

func (User) TableName() string { return "users" }

func UpsertUser(gdb *gorm.DB, appKey, userID, nickname, portrait string) error {
	var user User
	err := gdb.Where("app_key = ? AND user_id = ?", appKey, userID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return gdb.Create(&User{
			AppKey: appKey, UserID: userID, Nickname: nickname, UserPortrait: portrait,
		}).Error
	}
	if err != nil {
		return err
	}
	return gdb.Model(&user).Updates(map[string]any{
		"nickname": nickname, "user_portrait": portrait,
	}).Error
}

func FindUser(gdb *gorm.DB, appKey, userID string) (User, bool, error) {
	var user User
	err := gdb.Where("app_key = ? AND user_id = ?", appKey, userID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return user, true, nil
}
