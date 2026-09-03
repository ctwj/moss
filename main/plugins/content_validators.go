package plugins

import (
	"regexp"
	"unicode"
)

// 共享有效性校验器（"..." 标题与正文修复特性的唯一定义，research.md D3）。
// 恢复侧（FixEllipsisArticles）与写回侧（AISeoPlugin / GnDownSpider）必须共用本文件的判定，
// 禁止另行实现口径，避免两侧判定漂移。

const (
	// 默认阈值：标题可读字符（字母/数字/汉字）最少个数
	defaultMinTitleChars = 4
	// 默认阈值：正文剥 HTML 后可读字符最少个数
	defaultMinContentChars = 30
)

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

// IsPlaceholderText 判定文本是否为占位/无效文本：
// 去除空白与全部标点/符号后为空（覆盖 "..."、"…"、"。。。"、"---"、"???" 等任意纯符号串）。
func IsPlaceholderText(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		return false
	}
	return true
}

// CountAlphanumericHan 统计可读字符数（Unicode 字母，含汉字，与数字）。
func CountAlphanumericHan(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// IsValidArticleTitleWithMin 以指定阈值判定标题有效性：非占位且可读字符数达到 minChars。
func IsValidArticleTitleWithMin(s string, minChars int) bool {
	if minChars <= 0 {
		minChars = defaultMinTitleChars
	}
	return !IsPlaceholderText(s) && CountAlphanumericHan(s) >= minChars
}

// IsValidArticleTitle 以默认阈值（4）判定标题有效性。
func IsValidArticleTitle(s string) bool {
	return IsValidArticleTitleWithMin(s, defaultMinTitleChars)
}

// IsValidArticleContentWithMin 以指定阈值判定正文有效性（剥 HTML 标签后取纯文本判定）。
func IsValidArticleContentWithMin(html string, minChars int) bool {
	if minChars <= 0 {
		minChars = defaultMinContentChars
	}
	text := htmlTagPattern.ReplaceAllString(html, "")
	return !IsPlaceholderText(text) && CountAlphanumericHan(text) >= minChars
}

// IsValidArticleContent 以默认阈值（30）判定正文有效性。
func IsValidArticleContent(html string) bool {
	return IsValidArticleContentWithMin(html, defaultMinContentChars)
}
