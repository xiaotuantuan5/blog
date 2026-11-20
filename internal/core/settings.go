package core

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xiaotuantuan5/blog/internal/markdown"
)

// Settings are site-wide options shown on the public blog.
type Settings struct {
	SiteTitle       string `json:"siteTitle"`
	SiteDescription string `json:"siteDescription"`
	SiteURL         string `json:"siteUrl"` // used for RSS/sitemap absolute links
	Author          string `json:"author"`
	Avatar          string `json:"avatar"`
	AboutMD         string `json:"aboutMd"`
	AboutHTML       string `json:"aboutHtml"`
	PageSize        int    `json:"pageSize"`
}

var defaultSettings = Settings{
	SiteTitle:       "我的笔记",
	SiteDescription: "记录学习与思考",
	Author:          "",
	PageSize:        10,
}

func (a *App) GetSettings(ctx context.Context) (Settings, error) {
	s := defaultSettings
	rows, err := a.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		switch k {
		case "site_title":
			s.SiteTitle = v
		case "site_description":
			s.SiteDescription = v
		case "site_url":
			s.SiteURL = v
		case "author":
			s.Author = v
		case "avatar":
			s.Avatar = v
		case "about_md":
			s.AboutMD = v
		case "about_html":
			s.AboutHTML = v
		case "page_size":
			if n, err := strconv.Atoi(v); err == nil {
				s.PageSize = n
			}
		}
	}
	return s, rows.Err()
}

func (a *App) UpdateSettings(ctx context.Context, s Settings) (Settings, error) {
	s.SiteTitle = strings.TrimSpace(s.SiteTitle)
	s.SiteURL = strings.TrimRight(strings.TrimSpace(s.SiteURL), "/")
	if s.SiteTitle == "" {
		return s, invalid("站点名称不能为空")
	}
	if utf8.RuneCountInString(s.SiteTitle) > 100 {
		return s, invalid("站点名称太长")
	}
	if s.SiteURL != "" && !strings.HasPrefix(s.SiteURL, "http://") && !strings.HasPrefix(s.SiteURL, "https://") {
		return s, invalid("站点地址需以 http:// 或 https:// 开头")
	}
	if s.PageSize < 1 || s.PageSize > 100 {
		return s, invalid("每页条数需在 1 到 100 之间")
	}
	res, err := markdown.Render(s.AboutMD)
	if err != nil {
		return s, err
	}
	s.AboutHTML = res.HTML

	values := map[string]string{
		"site_title":       s.SiteTitle,
		"site_description": s.SiteDescription,
		"site_url":         s.SiteURL,
		"author":           s.Author,
		"avatar":           s.Avatar,
		"about_md":         s.AboutMD,
		"about_html":       s.AboutHTML,
		"page_size":        strconv.Itoa(s.PageSize),
	}
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		for k, v := range values {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
				return err
			}
		}
		return nil
	})
	return s, err
}
