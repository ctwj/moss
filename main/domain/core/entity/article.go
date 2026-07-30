package entity

import (
	"moss/domain/config"
	"moss/domain/core/vo"
	"strings"
	"time"
)

type Article struct {
	ArticleBase
	ArticleDetail
}

type ArticleBase struct {
	ID          int    `gorm:"type:int;size:32;primaryKey;autoIncrement" json:"id"`
	Slug        string `gorm:"type:varchar(150);uniqueIndex;not null"    json:"slug"`
	Title       string `gorm:"type:varchar(250);default:'';index"        json:"title"`
	CreateTime  int64  `gorm:"type:int;size:32;index"                    json:"create_time"`
	CategoryID  int    `gorm:"type:int;size:32;default:0;index"          json:"category_id"`
	Views       int    `gorm:"type:int;size:32;default:0;index"          json:"views"`
	Thumbnail   string `gorm:"type:varchar(250);default:''"              json:"thumbnail"`
	Description string `gorm:"type:varchar(250);default:''"              json:"description"`
	Status      bool   `gorm:"type:boolean;default:false;index"          json:"status"` // 发布状态 true:已发布 false:未发布
	DownloadPaused bool   `gorm:"type:boolean;default:false;index"          json:"download_paused"` // 下载暂停（版权下架） true:已暂停 false:正常
	GenuineURL     string `gorm:"type:varchar(500);default:''"              json:"genuine_url"`     // 正版页面 URL（「请支持正版」跳转目标）
}

func (ArticleBase) TableName() string {
	return "article"
}

func (a *ArticleBase) FullURL() string {
	return config.Config.Site.GetURL() + a.URL()
}

func (a *ArticleBase) URL() string {
	return strings.Replace(config.Config.Router.GetArticleRule(), ":slug", a.Slug, 1)
}

func (a *ArticleBase) CreateTimeFormat(layouts ...string) string {
	defer func() {
		if r := recover(); r != nil {
			// 捕获 panic，返回空字符串
		}
	}()
	if a.CreateTime == 0 {
		return ""
	}
	var layout = "2006-01-02 15:04:05"
	if len(layouts) > 0 && len(layouts[0]) > 0 {
		layout = layouts[0]
	}
	return time.Unix(a.CreateTime, 0).Format(layout)
}

type ArticleDetail struct {
	ArticleID int        `gorm:"type:int;size:32;primaryKey"   json:"article_id"`
	Keywords  string     `gorm:"type:varchar(250);default:''"  json:"keywords"`
	Content   string     `gorm:"type:string"                   json:"content"`
	Extends   vo.Extends `gorm:"type:string"                   json:"extends"`
	Res       vo.Extends `gorm:"type:string"                   json:"res"`
}
