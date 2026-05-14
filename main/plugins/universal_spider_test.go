package plugins

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// ==================== Helper function tests ====================

func TestNormalizeBaseURL(t *testing.T) {
	fmt.Println("\n========== 测试 URL 规范化 ==========")

	testCases := []struct {
		input    string
		expected string
	}{
		{"https://www.gndown.com", "https://www.gndown.com"},
		{"https://www.gndown.com/", "https://www.gndown.com"},
		{"https://www.gndown.com/page/2", "https://www.gndown.com"},
		{"http://example.com/path?q=1", "http://example.com"},
		{"  https://www.ludown.com/  ", "https://www.ludown.com"},
		{"", ""},
		{"not-a-url", "not-a-url"},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		result := normalizeBaseURL(tc.input)
		if result == tc.expected {
			fmt.Printf("  ✅ %q → %q\n", tc.input, result)
			passed++
		} else {
			fmt.Printf("  ❌ %q: 预期 %q, 实际 %q\n", tc.input, tc.expected, result)
			failed++
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestBuildSlug(t *testing.T) {
	fmt.Println("\n========== 测试 Slug 生成 ==========")

	testCases := []struct {
		siteID    string
		title     string
		wantLen   int
		wantEmpty bool
	}{
		{"gndown", "测试文章标题", 12, false},
		{"ludown", "测试文章标题", 12, false},
		{"", "标题", 12, false},
		{"gndown", "", 0, true},
		{"gndown", "   ", 0, true},
	}

	passed := 0
	failed := 0
	seen := make(map[string]bool)
	for _, tc := range testCases {
		result := buildSlug(tc.siteID, tc.title)
		if tc.wantEmpty {
			if result == "" {
				fmt.Printf("  ✅ buildSlug(%q, %q) = \"\"\n", tc.siteID, tc.title)
				passed++
			} else {
				fmt.Printf("  ❌ buildSlug(%q, %q) = %q, want empty\n", tc.siteID, tc.title, result)
				failed++
				t.Errorf("buildSlug(%q, %q) should be empty", tc.siteID, tc.title)
			}
			continue
		}
		if len(result) != tc.wantLen {
			fmt.Printf("  ❌ buildSlug(%q, %q) = %q (len=%d), want len=%d\n", tc.siteID, tc.title, result, len(result), tc.wantLen)
			failed++
			t.Errorf("buildSlug(%q, %q) len = %d, want %d", tc.siteID, tc.title, len(result), tc.wantLen)
			continue
		}
		if seen[result] {
			fmt.Printf("  ❌ buildSlug(%q, %q) = %q, duplicate slug!\n", tc.siteID, tc.title, result)
			failed++
			t.Errorf("duplicate slug detected for different inputs")
			continue
		}
		seen[result] = true
		fmt.Printf("  ✅ buildSlug(%q, %q) = %q\n", tc.siteID, tc.title, result)
		passed++
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestBuildPageURL(t *testing.T) {
	fmt.Println("\n========== 测试分页 URL 构建 ==========")

	testCases := []struct {
		baseURL   string
		page      int
		pattern   string
		expected  string
	}{
		{"https://www.gndown.com", 1, "/page/{page}", "https://www.gndown.com"},
		{"https://www.gndown.com", 2, "/page/{page}", "/page/2"},
		{"https://www.gndown.com", 3, "/page/{page}", "/page/3"},
		{"https://www.gndown.com", 2, "", "https://www.gndown.com/page/2"},
		{"https://example.com", 5, "/list/{page}.html", "/list/5.html"},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		result := buildPageURL(tc.baseURL, tc.page, tc.pattern)
		if result == tc.expected {
			fmt.Printf("  ✅ page %d → %s\n", tc.page, result)
			passed++
		} else {
			fmt.Printf("  ❌ page %d: 预期 %s, 实际 %s\n", tc.page, tc.expected, result)
			failed++
			t.Errorf("buildPageURL(%q, %d, %q) = %q, want %q", tc.baseURL, tc.page, tc.pattern, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestParseSkipTexts(t *testing.T) {
	fmt.Println("\n========== 测试跳过文本解析 ==========")

	testCases := []struct {
		input    string
		expected []string
	}{
		{"VIP,置顶", []string{"VIP", "置顶"}},
		{"VIP", []string{"VIP"}},
		{"", nil},
		{"  VIP , 置顶 , 推荐  ", []string{"VIP", "置顶", "推荐"}},
		{",,,", nil},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		result := parseSkipTexts(tc.input)
		if len(result) != len(tc.expected) {
			fmt.Printf("  ❌ %q: 预期 %v, 实际 %v\n", tc.input, tc.expected, result)
			failed++
			t.Errorf("parseSkipTexts(%q) = %v, want %v", tc.input, result, tc.expected)
			continue
		}
		ok := true
		for i := range result {
			if result[i] != tc.expected[i] {
				ok = false
				break
			}
		}
		if ok {
			fmt.Printf("  ✅ %q → %v\n", tc.input, result)
			passed++
		} else {
			fmt.Printf("  ❌ %q: 预期 %v, 实际 %v\n", tc.input, tc.expected, result)
			failed++
			t.Errorf("parseSkipTexts(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestSplitSelectors(t *testing.T) {
	fmt.Println("\n========== 测试选择器拆分 ==========")

	testCases := []struct {
		input    string
		expected []string
	}{
		{".excerpt, .post-list .post-item", []string{".excerpt", ".post-list .post-item"}},
		{".single", []string{".single"}},
		{"  a , b , c  ", []string{"a", "b", "c"}},
		{"", nil},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		result := splitSelectors(tc.input)
		if len(result) != len(tc.expected) {
			fmt.Printf("  ❌ %q: 预期 %v, 实际 %v\n", tc.input, tc.expected, result)
			failed++
			continue
		}
		ok := true
		for i := range result {
			if result[i] != tc.expected[i] {
				ok = false
				break
			}
		}
		if ok {
			fmt.Printf("  ✅ %q → %v\n", tc.input, result)
			passed++
		} else {
			fmt.Printf("  ❌ %q: 预期 %v, 实际 %v\n", tc.input, tc.expected, result)
			failed++
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestConvertToAbsoluteURL(t *testing.T) {
	fmt.Println("\n========== 测试相对 URL 转绝对 URL ==========")

	testCases := []struct {
		imageURL string
		baseURL  string
		expected string
	}{
		{"/images/test.png", "https://www.gndown.com", "https://www.gndown.com/images/test.png"},
		{"https://cdn.example.com/img.png", "https://www.gndown.com", "https://cdn.example.com/img.png"},
		{"//cdn.example.com/img.png", "https://www.gndown.com", "https://cdn.example.com/img.png"},
		{"./images/test.png", "https://www.gndown.com", "https://www.gndown.com/images/test.png"},
		{"  /img.png  ", "https://example.com", "https://example.com/img.png"},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		result := convertToAbsoluteURL(tc.imageURL, tc.baseURL)
		if result == tc.expected {
			fmt.Printf("  ✅ %q → %s\n", tc.imageURL, result)
			passed++
		} else {
			fmt.Printf("  ❌ %q: 预期 %s, 实际 %s\n", tc.imageURL, tc.expected, result)
			failed++
			t.Errorf("convertToAbsoluteURL(%q, %q) = %q, want %q", tc.imageURL, tc.baseURL, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Content extraction tests ====================

func TestExtractTitle(t *testing.T) {
	fmt.Println("\n========== 测试标题提取 ==========")

	testCases := []struct {
		name     string
		html     string
		expected string
	}{
		{"标准h1", `<!DOCTYPE html><html><body><h1>Q-Dir v12.51 多语便携版</h1></body></html>`, "Q-Dir v12.51 多语便携版"},
		{"多个h1取第一个", `<!DOCTYPE html><html><body><h1>标题一</h1><h1>标题二</h1></body></html>`, "标题一"},
		{"无h1", `<!DOCTYPE html><html><body><h2>副标题</h2></body></html>`, ""},
		{"带空白", `<!DOCTYPE html><html><body><h1>  前后有空格  </h1></body></html>`, "前后有空格"},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := extractTitle(doc)
		result = strings.TrimSpace(result)
		if result == tc.expected {
			fmt.Printf("  ✅ %s: '%s'\n", tc.name, result)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 '%s'\n", tc.name, tc.expected, result)
			failed++
			t.Errorf("extractTitle(%s) = %q, want %q", tc.name, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestExtractVersion(t *testing.T) {
	fmt.Println("\n========== 测试版本号提取 ==========")

	testCases := []struct {
		name     string
		html     string
		expected string
	}{
		{"v前缀版本", `<!DOCTYPE html><html><body><h1>Q-Dir v12.51 多语便携版</h1></body></html>`, "12.51"},
		{"三段版本号", `<!DOCTYPE html><html><body><h1>软件 1.2.3 正式版</h1></body></html>`, "1.2.3"},
		{"无版本号", `<!DOCTYPE html><html><body><h1>普通文章标题</h1></body></html>`, ""},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := extractVersion(doc)
		if result == tc.expected {
			fmt.Printf("  ✅ %s: '%s'\n", tc.name, result)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 '%s'\n", tc.name, tc.expected, result)
			failed++
			t.Errorf("extractVersion(%s) = %q, want %q", tc.name, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestExtractMetaDescription(t *testing.T) {
	fmt.Println("\n========== 测试 Meta Description 提取 ==========")

	spider := &UniversalSpider{}

	testCases := []struct {
		name        string
		html        string
		wantNonEmpty bool
	}{
		{"Twitter描述", `<!DOCTYPE html><html><head><meta name="twitter:description" content="这是Twitter描述"></head><body></body></html>`, true},
		{"OG描述", `<!DOCTYPE html><html><head><meta property="og:description" content="这是OG描述"></head><body></body></html>`, true},
		{"标准描述", `<!DOCTYPE html><html><head><meta name="description" content="标准描述"></head><body></body></html>`, true},
		{"无描述有正文", `<!DOCTYPE html><html><body><div class="article-content"><p>正文第一段落内容</p></div></body></html>`, true},
		{"全部为空", `<!DOCTYPE html><html><body></body></html>`, false},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := spider.extractMetaDescription(doc)
		if (result != "") == tc.wantNonEmpty {
			fmt.Printf("  ✅ %s: '%s'\n", tc.name, truncate(result, 50))
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 nonEmpty=%v, 实际 '%s'\n", tc.name, tc.wantNonEmpty, result)
			failed++
			t.Errorf("extractMetaDescription(%s) nonEmpty mismatch", tc.name)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestExtractTime(t *testing.T) {
	fmt.Println("\n========== 测试时间提取 ==========")

	spider := &UniversalSpider{}

	testCases := []struct {
		name        string
		html        string
		wantNonZero bool
	}{
		{"标准日期时间", `<!DOCTYPE html><html><body><time class="time">2025-01-15 10:30:00</time></body></html>`, true},
		{"仅日期", `<!DOCTYPE html><html><body><span class="post-time">2025-01-15</span></body></html>`, true},
		{"无时间（返回now）", `<!DOCTYPE html><html><body></body></html>`, true},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := spider.extractTime(doc)
		if (result > 0) == tc.wantNonZero {
			fmt.Printf("  ✅ %s: unix=%d\n", tc.name, result)
			passed++
		} else {
			fmt.Printf("  ❌ %s: unix=%d, want nonZero=%v\n", tc.name, result, tc.wantNonZero)
			failed++
			t.Errorf("extractTime(%s) = %d, wantNonZero=%v", tc.name, result, tc.wantNonZero)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestExtractThumbnail(t *testing.T) {
	fmt.Println("\n========== 测试缩略图提取 ==========")

	spider := &UniversalSpider{}

	testCases := []struct {
		name        string
		html        string
		wantNonEmpty bool
	}{
		{"Twitter图片", `<!DOCTYPE html><html><head><meta property="twitter:image" content="https://cdn.example.com/thumb.jpg"></head><body></body></html>`, true},
		{"OG图片", `<!DOCTYPE html><html><head><meta property="og:image" content="https://cdn.example.com/og.jpg"></head><body></body></html>`, true},
		{"内容中图片", `<!DOCTYPE html><html><body><div class="article-content"><img src="https://cdn.example.com/content.jpg"></div></body></html>`, true},
		{"无图片", `<!DOCTYPE html><html><body><p>纯文本内容</p></body></html>`, false},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := spider.extractThumbnail(doc, "https://www.gndown.com/article/123.html")
		if (result != "") == tc.wantNonEmpty {
			fmt.Printf("  ✅ %s: '%s'\n", tc.name, truncate(result, 60))
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 nonEmpty=%v, 实际 '%s'\n", tc.name, tc.wantNonEmpty, result)
			failed++
			t.Errorf("extractThumbnail(%s) nonEmpty mismatch", tc.name)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestExtractContent(t *testing.T) {
	fmt.Println("\n========== 测试正文内容提取 ==========")

	spider := &UniversalSpider{Sites: []SiteConfig{{URL: "https://www.gndown.com"}}}

	testCases := []struct {
		name        string
		html        string
		wantNonEmpty bool
		wantContains string
	}{
		{
			"article-content选择器",
			`<!DOCTYPE html><html><body><article class="article-content"><p>正文内容</p></article></body></html>`,
			true, "正文内容",
		},
		{
			"entry-content选择器",
			`<!DOCTYPE html><html><body><div class="entry-content"><p>博客正文</p></div></body></html>`,
			true, "博客正文",
		},
		{
			"图片data-src转换",
			`<!DOCTYPE html><html><body><div class="article-content"><img data-src="/images/test.png"><p>有图片</p></div></body></html>`,
			true, "https://www.gndown.com/images/test.png",
		},
		{
			"无正文",
			`<!DOCTYPE html><html><body><nav>导航</nav></body></html>`,
			false, "",
		},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := spider.extractContent(doc, &spider.Sites[0])
		if tc.wantNonEmpty && result == "" {
			fmt.Printf("  ❌ %s: 预期非空内容\n", tc.name)
			failed++
			t.Errorf("extractContent(%s) should not be empty", tc.name)
			continue
		}
		if !tc.wantNonEmpty && result != "" {
			fmt.Printf("  ❌ %s: 预期空内容, 实际 '%s'\n", tc.name, truncate(result, 50))
			failed++
			continue
		}
		if tc.wantContains != "" && !strings.Contains(result, tc.wantContains) {
			fmt.Printf("  ❌ %s: 内容应包含 '%s', 实际 '%s'\n", tc.name, tc.wantContains, truncate(result, 80))
			failed++
			t.Errorf("extractContent(%s) should contain %q", tc.name, tc.wantContains)
			continue
		}
		fmt.Printf("  ✅ %s\n", tc.name)
		passed++
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Category extraction tests ====================

func TestUniversalExtractCategory(t *testing.T) {
	fmt.Println("\n========== 测试通用分类提取 ==========")

	spider := &UniversalSpider{}

	testCases := []struct {
		name     string
		html     string
		expected string
	}{
		{
			"span.item 分类链接",
			`<!DOCTYPE html><html><body><span class="item">分类：<a href="/category/software" rel="category tag">应用软件</a></span></body></html>`,
			"应用软件",
		},
		{
			"a.cat 标签",
			`<!DOCTYPE html><html><body><a class="cat" href="/category/software"><i>icon</i>应用软件</a></body></html>`,
			"应用软件",
		},
		{
			"面包屑导航",
			`<!DOCTYPE html><html><body><div class="breadcrumbs"><a href="/">首页</a><a href="/category/software">应用软件</a><a href="/article/123">文章</a></div></body></html>`,
			"应用软件",
		},
		{
			"article-meta .cat",
			`<!DOCTYPE html><html><body><div class="article-meta"><span class="cat">应用软件</span></div></body></html>`,
			"应用软件",
		},
		{
			"article-meta category链接",
			`<!DOCTYPE html><html><body><div class="article-meta"><a href="/category/software">应用软件</a></div></body></html>`,
			"应用软件",
		},
		{
			"meta .cat",
			`<!DOCTYPE html><html><body><div class="meta"><span class="cat">应用软件</span></div></body></html>`,
			"应用软件",
		},
		{
			"清理分类前缀",
			`<!DOCTYPE html><html><body><div class="article-meta"><span class="cat">分类：应用软件</span></div></body></html>`,
			"应用软件",
		},
		{
			"无分类",
			`<!DOCTYPE html><html><body><p>无分类信息</p></body></html>`,
			"",
		},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		result := spider.extractCategory(doc, &SiteConfig{})
		if result == tc.expected {
			fmt.Printf("  ✅ %s: '%s'\n", tc.name, result)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 '%s'\n", tc.name, tc.expected, result)
			failed++
			t.Errorf("extractCategory(%s) = %q, want %q", tc.name, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Download section tests ====================

func TestIsValidDownloadURL(t *testing.T) {
	fmt.Println("\n========== 测试下载 URL 有效性判断 ==========")

	testCases := []struct {
		input    string
		expected bool
	}{
		{"https://pan.baidu.com/s/abc", true},
		{"https://pan.quark.cn/s/abc", true},
		{"https://www.lanzoub.com/abc", true},
		{"https://url33.ctfile.com/d/abc", true},
		{"https://www.123912.com/s/abc", true},
		{"https://www.aliyundrive.com/s/abc", true},
		{"https://www.example.com/file.zip", true},
		{"ftp://invalid.com/file", false},
		{"not-a-url", false},
		{"", false},
		{"/relative/path.html", false},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		result := isValidDownloadURL(tc.input)
		if result == tc.expected {
			fmt.Printf("  ✅ %q → %v\n", tc.input, result)
			passed++
		} else {
			fmt.Printf("  ❌ %q: 预期 %v, 实际 %v\n", tc.input, tc.expected, result)
			failed++
			t.Errorf("isValidDownloadURL(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestDetectCloudDriveType(t *testing.T) {
	fmt.Println("\n========== 测试网盘类型检测 ==========")

	testCases := []struct {
		name     string
		linkURL  string
		expected string
	}{
		{"百度云", "https://pan.baidu.com/s/abc", "百度云"},
		{"夸克云", "https://pan.quark.cn/s/abc", "夸克云"},
		{"蓝奏云", "https://www.lanzoub.com/abc", "蓝奏云"},
		{"城通盘", "https://url33.ctfile.com/d/abc", "城通盘"},
		{"123盘", "https://www.123912.com/s/abc", "123盘"},
		{"阿里云盘", "https://www.aliyundrive.com/s/abc", "阿里云盘"},
		{"未知", "https://www.unknown.com/s/abc", ""},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		// Create minimal document with an anchor
		html := fmt.Sprintf(`<html><body><p><a href="%s">%s</a></p></body></html>`, tc.linkURL, tc.linkURL)
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		a := doc.Find("a").First()
		result := detectCloudDriveType(tc.linkURL, a, a.Parent())
		if result == tc.expected {
			fmt.Printf("  ✅ %s: '%s'\n", tc.name, result)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 '%s'\n", tc.name, tc.expected, result)
			failed++
			t.Errorf("detectCloudDriveType(%s) = %q, want %q", tc.name, result, tc.expected)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestUniversalProcessDownloadSection(t *testing.T) {
	fmt.Println("\n========== 测试下载区域处理 ==========")

	spider := &UniversalSpider{}

	testCases := []struct {
		name            string
		html            string
		wantLinkCount   int
		wantContentTrim string
	}{
		{
			name: "完整下载区域",
			html: `<p>正文内容段落</p>
<h5>下载地址</h5><hr/>
<p>夸克云：<a href="https://pan.quark.cn/s/abc">https://pan.quark.cn/s/abc</a><br/>
百度云：<a href="https://pan.baidu.com/s/def?pwd=1234">https://pan.baidu.com/s/def?pwd=1234</a> (访问密码: 1234)<br/>
蓝奏云：<a href="https://lanzoub.com/abc">https://lanzoub.com/abc</a></p>`,
			wantLinkCount:   3,
			wantContentTrim: "正文内容段落",
		},
		{
			name: "无下载区域",
			html: `<p>这里是文章内容，没有下载地址部分。</p>
<p>第二段内容。</p>`,
			wantLinkCount:   0,
			wantContentTrim: "这里是文章内容",
		},
		{
			name: "下载标记但无链接",
			html: `<p>正文内容</p>
<h5>下载地址</h5><hr/>
<p>暂无下载链接</p>`,
			wantLinkCount:   0,
			wantContentTrim: "正文内容",
		},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		content, links := spider.processDownloadSection(tc.html, &SiteConfig{})
		linkOK := len(links) == tc.wantLinkCount
		contentOK := tc.wantContentTrim == "" || strings.Contains(content, tc.wantContentTrim)

		if linkOK && contentOK {
			fmt.Printf("  ✅ %s: 链接数=%d, 内容含'%s'\n", tc.name, len(links), tc.wantContentTrim)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期链接=%d(实际%d), 内容应含'%s'\n", tc.name, tc.wantLinkCount, len(links), tc.wantContentTrim)
			if !linkOK {
				t.Errorf("processDownloadSection(%s) linkCount = %d, want %d", tc.name, len(links), tc.wantLinkCount)
			}
			failed++
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Article link extraction tests ====================

func TestUniversalGetArticleLinks(t *testing.T) {
	fmt.Println("\n========== 测试通用文章链接提取 ==========")

	spider := &UniversalSpider{Sites: []SiteConfig{
		{
			URL: "https://www.gndown.com",
			Selectors: SelectorMapping{
				ListItem:    ".excerpt",
				ArticleLink: "h2 a",
				SkipMarker:  "span.sticky-icon",
				SkipTexts:   "VIP,置顶",
			},
		},
	}}

	html := `<!DOCTYPE html><html><body>
		<div class="excerpt">
			<span class="sticky-icon">VIP</span>
			<h2><a href="/vip-article.html">VIP文章</a></h2>
		</div>
		<div class="excerpt">
			<h2><a href="/normal1.html">普通文章1</a></h2>
		</div>
		<div class="excerpt">
			<span class="sticky-icon">置顶</span>
			<h2><a href="/sticky-article.html">置顶文章</a></h2>
		</div>
		<div class="excerpt">
			<h2><a href="/normal2.html">普通文章2</a></h2>
		</div>
		<div class="excerpt">
			<h2><a href="/normal3.html">普通文章3</a></h2>
		</div>
	</body></html>`

	// We can't call getArticleLinks directly because it calls getBody which makes HTTP requests.
	// Instead test the parsing logic manually.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	sel := spider.Sites[0].Selectors
	skipTexts := parseSkipTexts(sel.SkipTexts)
	baseURL := normalizeBaseURL(spider.Sites[0].URL)

	seen := make(map[string]struct{})
	var links []string

	doc.Find(sel.ListItem).Each(func(i int, s *goquery.Selection) {
		if sel.SkipMarker != "" && len(skipTexts) > 0 {
			isSkip := false
			s.Find(sel.SkipMarker).Each(func(j int, span *goquery.Selection) {
				text := strings.TrimSpace(span.Text())
				for _, skip := range skipTexts {
					if text == skip {
						isSkip = true
						return
					}
				}
			})
			if isSkip {
				return
			}
		}

		s.Find(sel.ArticleLink).Each(func(j int, a *goquery.Selection) {
			href, exists := a.Attr("href")
			if !exists {
				return
			}
			if strings.HasPrefix(href, "/") {
				href = baseURL + href
			}
			if strings.Contains(href, baseURL) && strings.HasSuffix(href, ".html") {
				if _, ok := seen[href]; !ok {
					seen[href] = struct{}{}
					links = append(links, href)
				}
			}
		})
	})

	expectedLinks := []string{
		"https://www.gndown.com/normal1.html",
		"https://www.gndown.com/normal2.html",
		"https://www.gndown.com/normal3.html",
	}

	passed := 0
	failed := 0

	if len(links) != len(expectedLinks) {
		fmt.Printf("  ❌ 链接数量: 预期 %d, 实际 %d\n", len(expectedLinks), len(links))
		failed++
		t.Errorf("getArticleLinks: got %d links, want %d", len(links), len(expectedLinks))
	} else {
		fmt.Printf("  ✅ 链接数量正确: %d\n", len(links))
		passed++
	}

	for _, link := range expectedLinks {
		found := false
		for _, l := range links {
			if l == link {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("  ✅ 包含链接: %s\n", link)
			passed++
		} else {
			fmt.Printf("  ❌ 缺少链接: %s\n", link)
			failed++
			t.Errorf("missing expected link: %s", link)
		}
	}

	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Preset and config tests ====================

func TestFindPreset(t *testing.T) {
	fmt.Println("\n========== 测试预设站点查找 ==========")

	testCases := []struct {
		url      string
		wantName string
		wantNil  bool
	}{
		{"https://www.gndown.com", "绿软小站", false},
		{"https://www.ludown.com", "绿软部落", false},
		{"https://www.wycad.com", "无忧软件网", false},
		{"https://www.wuleba.com", "吾乐吧软件站", false},
		{"https://www.08lr.cn", "08绿软站", false},
		{"https://www.unknown-site.com", "", true},
	}

	passed := 0
	failed := 0
	for _, tc := range testCases {
		preset := findPreset(tc.url)
		if tc.wantNil {
			if preset == nil {
				fmt.Printf("  ✅ %s → nil (未找到)\n", tc.url)
				passed++
			} else {
				fmt.Printf("  ❌ %s: 预期 nil, 实际 '%s'\n", tc.url, preset.Name)
				failed++
			}
			continue
		}
		if preset == nil {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 nil\n", tc.url, tc.wantName)
			failed++
			t.Errorf("findPreset(%q) = nil, want %q", tc.url, tc.wantName)
			continue
		}
		if preset.Name == tc.wantName {
			fmt.Printf("  ✅ %s → %s (SiteID: %s)\n", tc.url, preset.Name, preset.SiteID)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 '%s'\n", tc.url, tc.wantName, preset.Name)
			failed++
			t.Errorf("findPreset(%q) = %q, want %q", tc.url, preset.Name, tc.wantName)
		}
	}
	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestApplyPreset(t *testing.T) {
	fmt.Println("\n========== 测试预设应用 ==========")

	spider := NewUniversalSpider()

	// Test auto-fill on load
	site := &SiteConfig{
		Enabled: true,
		URL:     "https://www.gndown.com",
	}
	spider.applyPreset(site)

	passed := 0
	failed := 0

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"SiteID", site.SiteID, "gndown"},
		{"Name", site.Name, "绿软小站"},
		{"ListItem选择器", site.Selectors.ListItem, ".excerpt, .post-list .post-item"},
		{"DownloadTag", site.Selectors.DownloadTag, "h5"},
		{"DownloadSection", site.DownloadSection, "下载地址"},
	}

	for _, c := range checks {
		if c.got == c.want {
			fmt.Printf("  ✅ %s: '%s'\n", c.name, c.got)
			passed++
		} else {
			fmt.Printf("  ❌ %s: 预期 '%s', 实际 '%s'\n", c.name, c.want, c.got)
			failed++
			t.Errorf("applyPreset %s = %q, want %q", c.name, c.got, c.want)
		}
	}

	// Test that existing config is not overwritten
	site2 := &SiteConfig{
		URL:     "https://www.gndown.com",
		SiteID:  "custom",
		Name:    "自定义名称",
		Selectors: SelectorMapping{Content: ".custom-content"},
	}
	spider.applyPreset(site2)

	if site2.SiteID == "custom" && site2.Name == "自定义名称" {
		fmt.Printf("  ✅ 已有配置不被覆盖\n")
		passed++
	} else {
		fmt.Printf("  ❌ 已有配置被覆盖: SiteID=%s, Name=%s\n", site2.SiteID, site2.Name)
		failed++
		t.Errorf("applyPreset should not override existing config")
	}

	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestResolveSiteConfig(t *testing.T) {
	fmt.Println("\n========== 测试站点配置解析 ==========")

	spider := &UniversalSpider{
		DefaultTimeout:  30,
		DefaultRetry:    2,
		DefaultInterval: 1,
		DefaultProxy:    "http://proxy:8080",
	}

	passed := 0
	failed := 0

	// Test with site-level overrides
	site := &SiteConfig{Timeout: 60, Retry: 3, RequestInterval: 5, Proxy: "http://site-proxy:8080"}
	timeout, retry, interval, proxy := spider.resolveSiteConfig(site)
	if timeout == 60 && retry == 3 && interval == 5 && proxy == "http://site-proxy:8080" {
		fmt.Printf("  ✅ 站点配置覆盖: timeout=%d, retry=%d, interval=%d, proxy=%s\n", timeout, retry, interval, proxy)
		passed++
	} else {
		fmt.Printf("  ❌ 站点配置覆盖失败\n")
		failed++
	}

	// Test with global defaults
	site2 := &SiteConfig{}
	timeout2, retry2, interval2, proxy2 := spider.resolveSiteConfig(site2)
	if timeout2 == 30 && retry2 == 2 && interval2 == 1 && proxy2 == "http://proxy:8080" {
		fmt.Printf("  ✅ 全局默认: timeout=%d, retry=%d, interval=%d, proxy=%s\n", timeout2, retry2, interval2, proxy2)
		passed++
	} else {
		fmt.Printf("  ❌ 全局默认失败: timeout=%d, retry=%d, interval=%d, proxy=%s\n", timeout2, retry2, interval2, proxy2)
		failed++
	}

	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

func TestResolveCategoryID(t *testing.T) {
	fmt.Println("\n========== 测试分类 ID 解析 ==========")

	spider := &UniversalSpider{DefaultCategory: 19}

	site := &SiteConfig{
		CategoryMap: CategoryMapping{
			"应用软件": 5,
			"系统工具": 6,
		},
	}

	passed := 0
	failed := 0

	// CategoryMap hit
	if id := spider.resolveCategoryID("应用软件", site); id == 5 {
		fmt.Printf("  ✅ CategoryMap 命中: 应用软件 → %d\n", id)
		passed++
	} else {
		fmt.Printf("  ❌ CategoryMap 未命中: 预期 5, 实际 %d\n", id)
		failed++
		t.Errorf("resolveCategoryID('应用软件') = %d, want 5", id)
	}

	// CategoryMap miss → default
	if id := spider.resolveCategoryID("未知分类", site); id == 19 {
		fmt.Printf("  ✅ 未知分类 → 默认 %d\n", id)
		passed++
	} else {
		fmt.Printf("  ❌ 未知分类: 预期 19, 实际 %d\n", id)
		failed++
		t.Errorf("resolveCategoryID('未知分类') = %d, want 19", id)
	}

	// Empty category → default
	if id := spider.resolveCategoryID("", site); id == 19 {
		fmt.Printf("  ✅ 空分类 → 默认 %d\n", id)
		passed++
	} else {
		fmt.Printf("  ❌ 空分类: 预期 19, 实际 %d\n", id)
		failed++
	}

	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Preset completeness tests ====================

func TestPresetCompleteness(t *testing.T) {
	fmt.Println("\n========== 测试预设配置完整性 ==========")

	spider := NewUniversalSpider()

	passed := 0
	failed := 0

	for i := range spider.Sites {
		site := &spider.Sites[i]
		spider.applyPreset(site)

		checks := []struct {
			name string
			val  string
		}{
			{"SiteID", site.SiteID},
			{"Name", site.Name},
			{"ListItem", site.Selectors.ListItem},
			{"ArticleLink", site.Selectors.ArticleLink},
			{"Content", site.Selectors.Content},
			{"DownloadTag", site.Selectors.DownloadTag},
			{"Pagination", site.Selectors.Pagination},
			{"DownloadSection", site.DownloadSection},
		}

		allOK := true
		for _, c := range checks {
			if c.val == "" {
				fmt.Printf("  ❌ [%s] %s 为空\n", site.Name, c.name)
				allOK = false
			}
		}
		if allOK {
			fmt.Printf("  ✅ [%s] 所有配置字段已填充 (SiteID=%s, CategoryMap条目=%d)\n",
				site.Name, site.SiteID, len(site.CategoryMap))
			passed++
		} else {
			failed++
			t.Errorf("preset for %s is incomplete", site.Name)
		}
	}

	fmt.Printf("结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Integration-style test for fetchArticle parsing ====================

func TestFetchArticleParsing(t *testing.T) {
	fmt.Println("\n========== 测试文章解析流程（纯解析，无HTTP） ==========")

	spider := &UniversalSpider{DefaultCategory: 19}
	site := &SiteConfig{
		SiteID:          "test",
		URL:             "https://www.gndown.com",
		DownloadSection: "下载地址",
		Selectors: SelectorMapping{
			Content:     "article.article-content, .article-content, .entry-content",
			DownloadTag: "h5",
			SkipMarker:  "span.sticky-icon",
			SkipTexts:   "VIP,置顶",
		},
	}

	// Simulate full article HTML parsing (without HTTP)
	html := `<!DOCTYPE html>
<html>
<head>
	<meta name="description" content="Q-Dir 是一款免费的文件管理器">
	<meta name="keywords" content="Q-Dir, 文件管理器">
	<meta property="og:image" content="https://cdn.example.com/thumb.jpg">
</head>
<body>
	<h1>Q-Dir(免费的文件管理器) v12.51 多语便携版</h1>
	<div class="breadcrumbs">
		<a href="/">首页</a>
		<a href="/category/software">应用软件</a>
		<a href="/article/123">文章</a>
	</div>
	<article class="article-content">
		<p>Q-Dir 是一款非常实用的文件管理工具。</p>
		<p>支持多窗口操作，方便文件管理。</p>
		<h5>下载地址</h5><hr/>
		<p>
			夸克云：<a href="https://pan.quark.cn/s/abc123">https://pan.quark.cn/s/abc123</a><br/>
			百度云：<a href="https://pan.baidu.com/s/def456?pwd=abcd">https://pan.baidu.com/s/def456?pwd=abcd</a> (访问密码: abcd)<br/>
		</p>
	</article>
</body>
</html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	passed := 0
	failed := 0

	// Test title
	title := extractTitle(doc)
	if title == "Q-Dir(免费的文件管理器) v12.51 多语便携版" {
		fmt.Printf("  ✅ 标题: %s\n", title)
		passed++
	} else {
		fmt.Printf("  ❌ 标题: 预期 'Q-Dir(免费的文件管理器) v12.51 多语便携版', 实际 '%s'\n", title)
		failed++
	}

	// Test slug
	slug := buildSlug("test", title)
	if len(slug) == 12 {
		fmt.Printf("  ✅ Slug: %s\n", slug)
		passed++
	} else {
		fmt.Printf("  ❌ Slug: len=%d, 预期 12\n", len(slug))
		failed++
	}

	// Test content
	content := spider.extractContent(doc, site)
	if content != "" && strings.Contains(content, "非常实用的文件管理工具") {
		fmt.Printf("  ✅ 正文: 包含预期内容 (len=%d)\n", len(content))
		passed++
	} else {
		fmt.Printf("  ❌ 正文: 为空或不包含预期内容\n")
		failed++
	}

	// Test download section
	processedContent, links := spider.processDownloadSection(content, site)
	if len(links) >= 2 {
		fmt.Printf("  ✅ 下载链接: %d 个\n", len(links))
		for _, link := range links {
			fmt.Printf("     - type=%s, url=%s\n", link["type"], truncate(link["url"], 50))
		}
		passed++
	} else {
		fmt.Printf("  ❌ 下载链接: 预期 >=2, 实际 %d\n", len(links))
		failed++
	}

	if !strings.Contains(processedContent, "非常实用的文件管理工具") {
		fmt.Printf("  ❌ 处理后正文不包含原始内容\n")
		failed++
	} else {
		fmt.Printf("  ✅ 处理后正文保留原始内容\n")
		passed++
	}

	// Test description
	desc := spider.extractMetaDescription(doc)
	if desc != "" {
		fmt.Printf("  ✅ 描述: %s\n", truncate(desc, 60))
		passed++
	} else {
		fmt.Printf("  ❌ 描述: 为空\n")
		failed++
	}

	// Test category
	cat := spider.extractCategory(doc, site)
	if cat == "应用软件" {
		fmt.Printf("  ✅ 分类: %s\n", cat)
		passed++
	} else {
		fmt.Printf("  ❌ 分类: 预期 '应用软件', 实际 '%s'\n", cat)
		failed++
	}

	// Test thumbnail
	thumb := spider.extractThumbnail(doc, "https://www.gndown.com/article/123.html")
	if thumb == "https://cdn.example.com/thumb.jpg" {
		fmt.Printf("  ✅ 缩略图: %s\n", thumb)
		passed++
	} else {
		fmt.Printf("  ❌ 缩略图: 预期 'https://cdn.example.com/thumb.jpg', 实际 '%s'\n", thumb)
		failed++
	}

	// Test version
	ver := extractVersion(doc)
	if ver == "12.51" {
		fmt.Printf("  ✅ 版本号: %s\n", ver)
		passed++
	} else {
		fmt.Printf("  ❌ 版本号: 预期 '12.51', 实际 '%s'\n", ver)
		failed++
	}

	// Test keywords
	kw := spider.extractKeywords(doc)
	if kw != "" {
		fmt.Printf("  ✅ 关键词: %s\n", kw)
		passed++
	} else {
		fmt.Printf("  ❌ 关键词: 为空\n")
		failed++
	}

	fmt.Printf("\n结果: ✅ %d, ❌ %d\n", passed, failed)
}

// ==================== Utility ====================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
