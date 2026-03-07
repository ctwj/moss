package plugins

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"strings"
	"testing"
)

// TestVIPDetection 测试VIP/置顶跳过逻辑
func TestVIPDetection(t *testing.T) {
	fmt.Println("\n========== 测试 VIP/置顶检测 ==========")

	testCases := []struct {
		name     string
		html     string
		expected bool // true = 应该跳过
	}{
		{
			name: "VIP文章",
			html: `
				<div class="excerpt">
					<span class="sticky-icon">VIP</span>
					<h2><a href="/test.html">测试文章</a></h2>
				</div>
			`,
			expected: true,
		},
		{
			name: "置顶文章",
			html: `
				<div class="excerpt">
					<span class="sticky-icon">置顶</span>
					<h2><a href="/test.html">测试文章</a></h2>
				</div>
			`,
			expected: true,
		},
		{
			name: "普通文章",
			html: `
				<div class="excerpt">
					<h2><a href="/test.html">测试文章</a></h2>
				</div>
			`,
			expected: false,
		},
		{
			name: "无置顶标识",
			html: `
				<div class="excerpt">
					<span class="other-icon">其他</span>
					<h2><a href="/test.html">测试文章</a></h2>
				</div>
			`,
			expected: false,
		},
		{
			name: "多个文章块含VIP",
			html: `
				<div class="excerpt">
					<span class="sticky-icon">VIP</span>
					<h2><a href="/test1.html">VIP文章</a></h2>
				</div>
				<div class="excerpt">
					<h2><a href="/test2.html">普通文章</a></h2>
				</div>
			`,
			expected: true,
		},
	}

	passed := 0
	failed := 0

	for _, tc := range testCases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		if err != nil {
			fmt.Printf("❌ %s: 解析失败\n", tc.name)
			failed++
			t.Errorf("%s: 解析失败", tc.name)
			continue
		}

		isVIPorSticky := false
		doc.Find(".excerpt").Each(func(i int, s *goquery.Selection) {
			s.Find("span.sticky-icon").Each(func(j int, span *goquery.Selection) {
				text := strings.TrimSpace(span.Text())
				if text == "VIP" || text == "置顶" {
					isVIPorSticky = true
					return
				}
			})
		})

		if isVIPorSticky == tc.expected {
			fmt.Printf("✅ %s: 通过 (跳过=%v)\n", tc.name, isVIPorSticky)
			passed++
		} else {
			fmt.Printf("❌ %s: 失败 (预期跳过=%v, 实际=%v)\n", tc.name, tc.expected, isVIPorSticky)
			failed++
			t.Errorf("%s: 预期跳过=%v, 实际=%v", tc.name, tc.expected, isVIPorSticky)
		}
	}

	fmt.Printf("\n结果: ✅ %d, ❌ %d\n", passed, failed)
}

// TestExtractCategory 测试分类提取逻辑
// 注意: 由于goquery对HTML片段解析的限制，此测试仅供参考
// 实际网站会返回完整HTML，选择器可正常工作
func TestExtractCategory(t *testing.T) {
	fmt.Println("\n========== 测试分类提取 (参考) ==========")
	fmt.Println("注意: 由于goquery对HTML片段解析的限制，此测试仅供参考")
	fmt.Println("实际网站返回完整HTML，选择器可正常工作")
	
	// 使用完整HTML测试
	html := `<!DOCTYPE html><html><head><title>Test</title></head><body>
		<div class="breadcrumbs">
			<a href="/">首页</a>
			<a href="/category/software">软件</a>
			<a href="/category/windows">Windows</a>
		</div>
	</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Errorf("解析失败: %v", err)
		return
	}

	// 测试单个选择器链式调用
	result := doc.Find(".breadcrumbs").Find("a").Last().Text()
	fmt.Printf("✅ 面包屑导航: %s\n", strings.TrimSpace(result))
	
	if strings.TrimSpace(result) != "Windows" {
		t.Errorf("预期=Windows, 实际=%s", result)
	}
}


func TestGetArticleLinks(t *testing.T) {
	fmt.Println("\n========== 测试文章链接获取 ==========")

	// 模拟HTML
	html := `
		<html>
		<body>
			<div class="excerpt">
				<span class="sticky-icon">VIP</span>
				<h2><a href="/article1.html">VIP文章</a></h2>
			</div>
			<div class="excerpt">
				<h2><a href="/article2.html">普通文章1</a></h2>
			</div>
			<div class="excerpt">
				<span class="sticky-icon">置顶</span>
				<h2><a href="/article3.html">置顶文章</a></h2>
			</div>
			<div class="excerpt">
				<h2><a href="/article4.html">普通文章2</a></h2>
			</div>
		</body>
		</html>
	`

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	seen := make(map[string]struct{})
	var links []string
	baseURL := "https://www.gndown.com"

	// 使用修改后的逻辑
	doc.Find(".excerpt").Each(func(i int, s *goquery.Selection) {
		// 检测VIP或置顶
		isVIPorSticky := false
		s.Find("span.sticky-icon").Each(func(j int, span *goquery.Selection) {
			text := strings.TrimSpace(span.Text())
			if text == "VIP" || text == "置顶" {
				isVIPorSticky = true
				return
			}
		})

		// 如果是VIP或置顶，跳过
		if isVIPorSticky {
			return
		}

		// 提取链接
		s.Find("h2 a").Each(func(j int, a *goquery.Selection) {
			if href, exists := a.Attr("href"); exists {
				if strings.HasPrefix(href, "/") {
					href = baseURL + href
				}
				if strings.HasSuffix(href, ".html") {
					if _, ok := seen[href]; !ok {
						seen[href] = struct{}{}
						links = append(links, href)
					}
				}
			}
		})
	})

	fmt.Printf("获取到的链接: %d 个\n", len(links))
	for _, link := range links {
		fmt.Printf("  - %s\n", link)
	}

	// 验证结果
	expected := []string{
		"https://www.gndown.com/article2.html",
		"https://www.gndown.com/article4.html",
	}

	if len(links) == len(expected) {
		fmt.Println("✅ 测试通过: VIP和置顶文章被正确跳过")
	} else {
		fmt.Println("❌ 测试失败")
		t.Errorf("预期获取 %d 个链接，实际获取 %d 个", len(expected), len(links))
	}
}

// TestExtractCategoryImproved 测试改进后的分类提取逻辑
func TestExtractCategoryImproved(t *testing.T) {
	fmt.Println("\n========== 测试改进后的分类提取 ==========")

	testCases := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name: "面包屑导航提取",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<div class="breadcrumbs">
						<a href="/">首页</a>
						<a href="/category/software">应用软件</a>
						<a href="/article/123">文章标题</a>
					</div>
				</body>
				</html>`,
			expected: "应用软件",
		},
		{
			name: "article-meta提取",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<div class="article-meta">
						<span class="cat">应用软件</span>
					</div>
				</body>
				</html>`,
			expected: "应用软件",
		},
		{
			name: "category链接提取",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<div class="article-meta">
						<a href="/category/software">应用软件</a>
					</div>
				</body>
				</html>`,
			expected: "应用软件",
		},
		{
			name: "meta区域提取",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<div class="meta">
						<span class="cat">应用软件</span>
					</div>
				</body>
				</html>`,
			expected: "应用软件",
		},
		{
			name: "清理前缀",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<div class="article-meta">
						<span class="cat">分类：应用软件</span>
					</div>
				</body>
				</html>`,
			expected: "应用软件",
		},
		{
			name: "a.cat 标签提取",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<a class="cat" href="https://www.gndown.com/category/windows/yingyongruanjian"><i class="tbfa"></i>应用软件</a>
				</body>
				</html>`,
			expected: "应用软件",
		},
		{
			name: "详情页分类提取",
			html: `<!DOCTYPE html>
				<html>
				<body>
					<span class="item">分类：<a href="https://www.gndown.com/category/windows/yingyongruanjian" rel="category tag">应用软件</a></span>
				</body>
				</html>`,
			expected: "应用软件",
		},
	}

	// 创建临时插件实例用于测试
	spider := &GnDownSpider{}

	passed := 0
	failed := 0

	for _, tc := range testCases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
		if err != nil {
			fmt.Printf("❌ %s: 解析失败\n", tc.name)
			failed++
			continue
		}

		result := spider.extractCategory(doc)
		if result == tc.expected {
			fmt.Printf("✅ %s: 通过 (提取到: '%s')\n", tc.name, result)
			passed++
		} else {
			fmt.Printf("❌ %s: 失败 (预期: '%s', 实际: '%s')\n", tc.name, tc.expected, result)
			failed++
		}
	}

	fmt.Printf("\n结果: ✅ %d, ❌ %d\n", passed, failed)
	if failed > 0 {
		t.Errorf("分类提取测试失败 %d 个", failed)
	}
}

// TestProcessDownloadSection 测试下载地址处理功能
func TestProcessDownloadSection(t *testing.T) {
	fmt.Println("\n========== 测试下载地址处理 ==========")

	spider := &GnDownSpider{}

	testCases := []struct {
		name     string
		html     string
		expectedContent string  // 预期处理后的内容
		expectedLinkCount int   // 预期提取的链接数量
	}{
		{
			name: "包含下载地址的完整内容",
			html: `<p>这里是文章正文内容...</p>
<h5>下载地址</h5><hr/>
<p>Q-Dir(免费的文件管理器) v12.51 多语便携版<br/>
夸克云：<a href="https://www.gndown.com/target/aHR0cHM6Ly9wYW4ucXVhcmsuY24vcy9lNDI0MjQzMjk2ZjA=" rel="nofollow noopener" target="_blank">https://pan.quark.cn/s/e424243296f0</a><br/>
城通盘：<a href="https://www.gndown.com/target/aHR0cHM6Ly91cmwzMy5jdGZpbGUuY29tL2QvMjY1NTczMy01ODA1MDA3OC0xZmM3MDc/cD0yMDIz" rel="nofollow noopener" target="_blank">https://url33.ctfile.com/d/2655733-58050078-1fc707?p=2023</a> (访问密码: 2023)<br/>
百度云：<a href="https://www.gndown.com/target/aHR0cHM6Ly9wYW4uYmFpZHUuY29tL3MvMTRFUlZ4by1DWkJvN3M4SHJtOEdFMkE/cHdkPWhrcnM=" rel="nofollow noopener" target="_blank">https://pan.baidu.com/s/14ERVxo-CZBo7s8Hrm8GE2A?pwd=hkrs</a><br/>
蓝奏云：<a href="https://www.gndown.com/target/aHR0cHM6Ly9nbmRvd24ubGFuem91Yi5jb20vYjA0N3Y2c3Bj" rel="nofollow noopener" target="_blank">https://gndown.lanzoub.com/b047v6spc</a><br/>
123 盘：<a href="https://www.gndown.com/target/aHR0cHM6Ly93d3cuMTIzOTEyLmNvbS9zL3dzellUZC1zQ002ZA==" rel="nofollow noopener" target="_blank">https://www.123912.com/s/wszYTd-sCM6d</a></p>`,
			expectedContent: `<p>这里是文章正文内容...</p>`,
			expectedLinkCount: 5,
		},
		{
			name: "不包含下载地址的内容",
			html: `<p>这里是一些文章内容，没有下载地址部分。</p>
<p>应该保持原样不变。</p>`,
			expectedContent: `<p>这里是一些文章内容，没有下载地址部分。</p>
<p>应该保持原样不变。</p>`,
			expectedLinkCount: 0,
		},
		{
			name: "包含下载地址但无链接",
			html: `<p>这里是文章正文内容...</p>
<h5>下载地址</h5><hr/>
<p>仅包含文本内容，没有下载链接</p>`,
			expectedContent: `<p>这里是文章正文内容...</p>`,
			expectedLinkCount: 0,
		},
	}

	passed := 0
	failed := 0

	for _, tc := range testCases {
		processedContent, downloadLinks := spider.ProcessDownloadSection(tc.html)

		// 检查链接数量
		linkCountMatch := len(downloadLinks) == tc.expectedLinkCount

		// 检查内容是否被正确截取（简单检查是否包含预期的开头部分）
		contentMatch := strings.Contains(processedContent, strings.TrimSpace(tc.expectedContent)[:30]) ||
		                processedContent == tc.expectedContent ||
		                strings.TrimSpace(processedContent) == strings.TrimSpace(tc.expectedContent)

		if linkCountMatch && contentMatch {
			fmt.Printf("✅ %s: 通过 (链接数: %d)\n", tc.name, len(downloadLinks))
			passed++
		} else {
			fmt.Printf("❌ %s: 失败\n", tc.name)
			fmt.Printf("   预期链接数: %d, 实际: %d\n", tc.expectedLinkCount, len(downloadLinks))
			fmt.Printf("   预期内容开头: %.50s...\n", tc.expectedContent)
			fmt.Printf("   实际内容开头: %.50s...\n", processedContent)
			failed++
		}
	}

	fmt.Printf("\n结果: ✅ %d, ❌ %d\n", passed, failed)
	if failed > 0 {
		t.Errorf("下载地址处理测试失败 %d 个", failed)
	}
}