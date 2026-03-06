package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"strings"
)

func main() {
	// 模拟测试HTML
	html := `<!DOCTYPE html>
<html>
<head><title>WizTree(磁盘空间分析软件) v4.30 多语便携版</title></head>
<body>
	<div class="breadcrumbs">
		<a href="/">首页</a>
		<a href="/category/software">应用软件</a>
		<a href="/category/utilities">工具软件</a>
	</div>
	<article>
		<h1>WizTree(磁盘空间分析软件) v4.30 多语便携版</h1>
		<div class="article-meta">
			<span class="cat">应用软件</span>
		</div>
		<div class="article-content">
			<p>文章内容...</p>
		</div>
	</article>
</body>
</html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		fmt.Printf("解析失败: %v\n", err)
		return
	}

	// 测试不同的选择器
	fmt.Println("=== 分类提取测试 ===")

	// 方法1: 面包屑导航最后一个
	breadcrumbCat := doc.Find(".breadcrumbs a:last").Text()
	fmt.Printf("面包屑最后一个: '%s'\n", strings.TrimSpace(breadcrumbCat))

	// 方法2: article-meta中的cat
	metaCat := doc.Find(".article-meta .cat").Text()
	fmt.Printf("article-meta cat: '%s'\n", strings.TrimSpace(metaCat))

	// 方法3: meta中的category链接
	metaLinkCat := doc.Find(".article-meta a[href*='category']").Text()
	fmt.Printf("article-meta category链接: '%s'\n", strings.TrimSpace(metaLinkCat))

	// 方法4: 综合结果（当前插件逻辑）
	category := doc.Find(".breadcrumbs a:last, .article-meta .cat, .meta .cat, .article-meta a[href*='category']").Last().Text()
	category = strings.TrimSpace(category)
	category = strings.TrimPrefix(category, "分类：")
	category = strings.TrimPrefix(category, "Category:")
	fmt.Printf("最终提取结果: '%s'\n", category)

	fmt.Printf("是否为空: %v\n", category == "")
	fmt.Printf("长度: %d\n", len(category))
}