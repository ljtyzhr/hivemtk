package model

import (
	"time"

	"gorm.io/gorm"
)

// SmsConfig 短信配置模型
type SmsConfig struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID              uint      `gorm:"primarykey" json:"id"`
	DefaultProvider string    `gorm:"type:varchar(50);default:'aliyun'" json:"defaultProvider"`
	RateLimit       int       `gorm:"default:100" json:"rateLimit"`
	DailyLimit      int       `gorm:"default:10000" json:"dailyLimit"`
	RetryTimes      int       `gorm:"default:3" json:"retryTimes"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// SmsAliyunConfig 阿里云短信配置
type SmsAliyunConfig struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID              uint      `gorm:"primarykey" json:"id"`
	AccessKeyID     string    `gorm:"type:varchar(100)" json:"accessKeyId"`
	AccessKeySecret string    `gorm:"type:varchar(100)" json:"accessKeySecret"`
	SignName        string    `gorm:"type:varchar(50)" json:"signName"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// SmsTencentConfig 腾讯云短信配置
type SmsTencentConfig struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        uint      `gorm:"primarykey" json:"id"`
	SecretID  string    `gorm:"type:varchar(100)" json:"secretId"`
	SecretKey string    `gorm:"type:varchar(100)" json:"secretKey"`
	AppID     string    `gorm:"type:varchar(50)" json:"appId"`
	SignName  string    `gorm:"type:varchar(50)" json:"signName"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SmsHuaweiConfig 华为云短信配置
type SmsHuaweiConfig struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        uint      `gorm:"primarykey" json:"id"`
	AppKey    string    `gorm:"type:varchar(100)" json:"appKey"`
	AppSecret string    `gorm:"type:varchar(100)" json:"appSecret"`
	Sender    string    `gorm:"type:varchar(50)" json:"sender"`
	Signature string    `gorm:"type:varchar(50)" json:"signature"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SmsRecord 短信发送记录
type SmsRecord struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID       uint   `gorm:"primarykey" json:"id"`
	Phone    string `gorm:"type:varchar(20);index" json:"phone"`
	Content  string `gorm:"type:text" json:"content"`
	Provider string `gorm:"type:varchar(50)" json:"provider"`
	// MsgID 运营商消息号（阿里云 BizId／腾讯云 SerialNo／华为云 smsId）。
	// 送达回执 webhook 以它回连并回写台账行；空串 = 渠道没给或改造前老行，
	// 此时回执只留在 sms_delivery_statuses 侧，台账行停在 sent。
	MsgID     string         `gorm:"type:varchar(64);index" json:"msgId,omitempty"`
	Status    string         `gorm:"type:varchar(20);index;default:'pending'" json:"status"`
	ErrorCode string         `gorm:"type:varchar(50)" json:"errorCode"`
	ErrorMsg  string         `gorm:"type:text" json:"errorMsg"`
	SendTime  *time.Time     `json:"sendTime"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// SmsDraft 短信草稿
type SmsDraft struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        uint           `gorm:"primarykey" json:"id"`
	Title     string         `gorm:"type:varchar(100);index" json:"title"`
	Content   string         `gorm:"type:text" json:"content"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// SmsJob 短信发送任务
type SmsJob struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID           uint           `gorm:"primarykey" json:"id"`
	Name         string         `gorm:"type:varchar(100);index" json:"name"`
	Total        int            `gorm:"default:0" json:"total"`
	Sent         int            `gorm:"default:0" json:"sent"`
	Failed       int            `gorm:"default:0" json:"failed"`
	Status       string         `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ScheduleTime *time.Time     `json:"scheduleTime"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// SmsJobDetail 短信任务详情
type SmsJobDetail struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        uint       `gorm:"primarykey" json:"id"`
	JobID     uint       `gorm:"index" json:"jobId"`
	Phone     string     `gorm:"type:varchar(20)" json:"phone"`
	Content   string     `gorm:"type:text" json:"content"`
	Status    string     `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ErrorCode string     `gorm:"type:varchar(50)" json:"errorCode"`
	ErrorMsg  string     `gorm:"type:text" json:"errorMsg"`
	SendTime  *time.Time `json:"sendTime"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
