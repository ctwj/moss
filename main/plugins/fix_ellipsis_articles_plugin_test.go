package plugins

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"moss/domain/core/entity"
	"moss/domain/core/vo"
)

// ---- extractSourceTitle ----

func TestExtractSourceTitle(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<html><body>
			<div class="breadcrumb">首页</div>
			<h1>  微信WeChat v4.0 电脑版下载  </h1>
			<h2>副标题不应被取到</h2>
		</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	got := extractSourceTitle(doc)
	if got != "微信WeChat v4.0 电脑版下载" {
		t.Errorf("extractSourceTitle = %q, want %q", got, "微信WeChat v4.0 电脑版下载")
	}

	empty, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><p>无标题</p></body></html>`))
	if got := extractSourceTitle(empty); got != "" {
		t.Errorf("无 h1 时应返回空, got %q", got)
	}
}

// ---- extractSourceContent ----

const ellipsisSourceFixture = `
<html><body>
	<article class="article-content">
		<script>var ad = 1;</script>
		<p>这是一款非常好用的电脑软件，支持多开与防撤回功能，绿色免安装。</p>
		<div class="related">相关推荐不应出现在正文中</div>
		<img data-src="/upload/img/cover.png" class="perfmatters-lazy" loading="lazy">
		<img src="https://cdn.example.com/pic.jpg">
		<p>软件界面简洁，运行流畅，值得下载使用。</p>
	</article>
	<div class="post-content"><p>这是备用容器内容，不应被取到。</p></div>
</body></html>`

func TestExtractSourceContent(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(ellipsisSourceFixture))
	if err != nil {
		t.Fatal(err)
	}
	baseURL := mustParseURL(t, "https://www.gndown.com/article/123.html")
	got := extractSourceContent(doc, baseURL)

	// 命中第一优先容器
	if !strings.Contains(got, "支持多开与防撤回功能") {
		t.Error("应包含主容器正文文本")
	}
	if strings.Contains(got, "备用容器内容") {
		t.Error("不应取到备用容器")
	}
	// 非正文元素被清理
	if strings.Contains(got, "相关推荐") {
		t.Error(".related 应被移除")
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "var ad") {
		t.Error("script 应被移除")
	}
	// data-src → src 且相对地址绝对化
	if !strings.Contains(got, `src="https://www.gndown.com/upload/img/cover.png"`) {
		t.Errorf("data-src 应转换为绝对地址, got: %s", got)
	}
	if strings.Contains(got, "data-src") {
		t.Error("data-src 属性应被移除")
	}
	if strings.Contains(got, "perfmatters-lazy") {
		t.Error("懒加载 class 应被移除")
	}
	// 已是绝对地址的图片保持不变
	if !strings.Contains(got, `src="https://cdn.example.com/pic.jpg"`) {
		t.Error("绝对地址图片应保持不变")
	}
}

func TestExtractSourceContent_FallbackContainer(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<html><body><div class="post-content">
			<p>备用容器：这款软件功能强大，值得下载使用，运行稳定流畅。</p>
		</div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	got := extractSourceContent(doc, mustParseURL(t, "https://www.gndown.com/a.html"))
	if !strings.Contains(got, "这款软件功能强大") {
		t.Errorf("应回退到 .post-content 容器, got: %s", got)
	}
}

// ---- buildDownloadRes ----

func TestBuildDownloadRes(t *testing.T) {
	if res := buildDownloadRes(nil); len(res) != 0 {
		t.Errorf("无链接时 Res 应为空, got %v", res)
	}

	links := []map[string]string{{"type": "百度网盘", "url": "https://pan.baidu.com/s/abc"}}
	res := buildDownloadRes(links)
	if len(res) != 1 || res[0].Key != "download_links" {
		t.Fatalf("Res 应含 download_links 项, got %v", res)
	}
	got, ok := res[0].Value.([]map[string]string)
	if !ok || len(got) != 1 || got[0]["url"] != "https://pan.baidu.com/s/abc" {
		t.Errorf("download_links 值不符, got %v", res[0].Value)
	}
}

// ---- 下载区段剥离与提取（口径与采集一致的 ProcessDownloadSection） ----

func TestProcessDownloadSectionRebuild(t *testing.T) {
	content := `<p>这是一款非常好用的电脑软件，支持多开与防撤回，绿色免安装推荐下载。</p>
		<h5>下载地址</h5>
		<p>百度网盘：<a href="https://pan.baidu.com/s/xyz">https://pan.baidu.com/s/xyz</a> 访问密码：ab12</p>
		<p>此行及之后的内容应被移除</p>`

	spider := &GnDownSpider{}
	processed, links := spider.ProcessDownloadSection(content)

	if !strings.Contains(processed, "支持多开与防撤回") {
		t.Error("下载区段之前的正文应保留")
	}
	if strings.Contains(processed, "下载地址") || strings.Contains(processed, "此行及之后") {
		t.Error("下载区段及之后的内容应被剥离")
	}
	if len(links) != 1 || links[0]["url"] != "https://pan.baidu.com/s/xyz" {
		t.Fatalf("应提取到 1 条下载链接, got %v", links)
	}
	if links[0]["type"] != "百度网盘" {
		t.Errorf("链接类型应为 百度网盘, got %q", links[0]["type"])
	}
	if links[0]["password"] != "ab12" {
		t.Errorf("应提取到访问密码 ab12, got %q", links[0]["password"])
	}
}

// ---- repairOneReason 门控（不触达数据库的路径） ----

func TestRepairOneReason_IdempotentSkip(t *testing.T) {
	p := NewFixEllipsisArticles()
	item := &entity.Article{}
	item.Title = "微信WeChat电脑版下载" // 正常标题
	item.Extends = vo.Extends{{Key: "source_url", Value: "https://www.gndown.com/a.html"}}

	status, reason := p.repairOneReason(item)
	if status != repairStatusSkipped || reason != ellipsisReasonNoPlaceholder {
		t.Errorf("正常标题应 skipped/no_placeholder, got %s/%s", status, reason)
	}
}

func TestRepairOneReason_NoSourceURL(t *testing.T) {
	p := NewFixEllipsisArticles()
	item := &entity.Article{}
	item.Title = "..." // 占位标题
	item.Extends = nil // 无源站地址

	status, reason := p.repairOneReason(item)
	if status != repairStatusFail || reason != ellipsisReasonNoSourceURL {
		t.Errorf("无源地址应 fail/no_source_url, got %s/%s", status, reason)
	}
	// 原状断言：标题不得被改动
	if item.Title != "..." {
		t.Errorf("失败路径不得改动标题, got %q", item.Title)
	}
}

// ---- 候选粗筛集合与校验器终审的一致性 ----

// TestPlaceholderTitlesAllInvalid SQL 粗筛集合（repository.placeholderTitles 同源）中的
// 占位标题必须全部被校验器判定无效，保证「粗筛命中 → 终审不误放行」。
func TestPlaceholderTitlesAllInvalid(t *testing.T) {
	for _, s := range []string{"...", "…", "。。。", "....", "..", ". . ."} {
		if IsValidArticleTitle(s) {
			t.Errorf("粗筛集合中的占位标题 %q 应被校验器判定无效", s)
		}
	}
}
