package plugins

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"moss/domain/core/entity"
)

const testBrokenDomain = "image.08rj.com"

// ---- extractThumbnailImage ----

// TestExtractThumbnailImage 缩略图提取：og:image 优先 → twitter:image 回退 → 正文第一张图回退
func TestExtractThumbnailImage(t *testing.T) {
	baseURL := mustParseURL(t, "https://www.gndown.com/some/article.html")

	testCases := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name: "og:image 命中",
			html: `<html><head><meta property="og:image" content="https://cdn.example.com/cover.jpg"></head>
				<body><div class="article-content"><img src="https://cdn.example.com/body.png"></div></body></html>`,
			expected: "https://cdn.example.com/cover.jpg",
		},
		{
			name: "无 og 时回退 twitter:image",
			html: `<html><head><meta name="twitter:image" content="https://cdn.example.com/tw.jpg"></head><body></body></html>`,
			expected: "https://cdn.example.com/tw.jpg",
		},
		{
			name: "无 meta 时回退正文第一张图",
			html: `<html><body><div class="article-content"><p>text</p><img src="https://cdn.example.com/first.png"><img src="https://cdn.example.com/second.png"></div></body></html>`,
			expected: "https://cdn.example.com/first.png",
		},
		{
			name: "相对地址按源页面绝对化",
			html: `<html><head><meta property="og:image" content="/wp-content/uploads/pic.jpg"></head><body></body></html>`,
			expected: "https://www.gndown.com/wp-content/uploads/pic.jpg",
		},
		{
			name: "og 指向失效域名时跳过并回退正文首图",
			html: `<html><head><meta property="og:image" content="https://image.08rj.com/dead.jpg"></head>
				<body><div class="entry-content"><img src="https://cdn.example.com/live.png"></div></body></html>`,
			expected: "https://cdn.example.com/live.png",
		},
		{
			name: "无任何候选返回空",
			html: `<html><body><p>只有文字</p></body></html>`,
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := newTestDoc(t, tc.html)
			got := extractThumbnailImage(doc, baseURL, testBrokenDomain)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// TestExtractThumbnailImageLazyAttr img 惰性加载属性（data-src/data-lazy-src）回退
func TestExtractThumbnailImageLazyAttr(t *testing.T) {
	baseURL := mustParseURL(t, "https://www.gndown.com/a.html")
	doc := newTestDoc(t, `<html><body><div class="post-content"><img data-src="https://cdn.example.com/lazy.png"></div></body></html>`)
	if got := extractThumbnailImage(doc, baseURL, testBrokenDomain); got != "https://cdn.example.com/lazy.png" {
		t.Fatalf("expected lazy attr fallback, got %q", got)
	}
}

// ---- extractContentImages ----

// TestExtractContentImages 正文图片提取：顺序保持、候选过滤
func TestExtractContentImages(t *testing.T) {
	baseURL := mustParseURL(t, "https://www.gndown.com/a.html")
	html := `<html><body>
		<div class="article-content">
			<img src="https://image.08rj.com/dead1.jpg">
			<img src="/relative/pic2.png">
			<img src="https://cdn.example.com/pic3.webp">
			<img data-src="https://cdn.example.com/pic4.jpg">
			<img src="//cdn.example.com/protocol-relative.jpg">
		</div>
	</body></html>`
	got := extractContentImages(newTestDoc(t, html), baseURL, testBrokenDomain)

	// 失效域名候选被过滤；协议相对地址可绝对化；data-src 可提取
	expected := []string{
		"https://www.gndown.com/relative/pic2.png",
		"https://cdn.example.com/pic3.webp",
		"https://cdn.example.com/pic4.jpg",
		"https://cdn.example.com/protocol-relative.jpg",
	}
	if len(got) != len(expected) {
		t.Fatalf("expected %d images, got %d: %v", len(expected), len(got), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], got[i])
		}
	}
}

// TestAbsolutize 地址绝对化
func TestAbsolutize(t *testing.T) {
	baseURL := mustParseURL(t, "https://www.gndown.com/dir/page.html")

	cases := []struct{ raw, expected string }{
		{"https://other.com/a.jpg", "https://other.com/a.jpg"},          // 已是绝对地址
		{"/img/a.jpg", "https://www.gndown.com/img/a.jpg"},              // 根相对
		{"b.jpg", "https://www.gndown.com/dir/b.jpg"},                   // 同目录相对
		{"", ""},                                                        // 空值原样返回
	}
	for _, c := range cases {
		if got := absolutize(baseURL, c.raw); got != c.expected {
			t.Fatalf("absolutize(%q) = %q, want %q", c.raw, got, c.expected)
		}
	}
}

// ---- planContentReplacement / applyContentReplacements ----

// TestPlanAndApplyContentReplacement 替换映射：按序一一对应、非失效 img 原样保留
func TestPlanAndApplyContentReplacement(t *testing.T) {
	content := `<p>开头</p><img src="https://image.08rj.com/dead1.jpg"><p>中间</p>` +
		`<img src="https://cdn.good.com/keep.png"><img src="https://image.08rj.com/dead2.jpg"><p>结尾</p>`
	sourceImages := []string{
		"https://cdn.new.com/new1.jpg",
		"https://cdn.new.com/new2.jpg",
		"https://cdn.new.com/new3.jpg",
	}

	plan, err := planContentReplacement(content, testBrokenDomain, sourceImages)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if len(plan) != 2 || plan[0] != sourceImages[0] || plan[1] != sourceImages[1] {
		t.Fatalf("unexpected plan: %v", plan)
	}

	newHTML, err := applyContentReplacements(content, testBrokenDomain, plan)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	// 失效地址全部替换，且按序映射
	if !strings.Contains(newHTML, `src="https://cdn.new.com/new1.jpg"`) ||
		!strings.Contains(newHTML, `src="https://cdn.new.com/new2.jpg"`) {
		t.Fatalf("replacement urls missing:\n%s", newHTML)
	}
	// 非失效 img 未被改动
	if !strings.Contains(newHTML, `src="https://cdn.good.com/keep.png"`) {
		t.Fatalf("non-broken img was modified:\n%s", newHTML)
	}
	// 正文文字保留
	for _, fragment := range []string{"开头", "中间", "结尾"} {
		if !strings.Contains(newHTML, fragment) {
			t.Fatalf("text %q missing:\n%s", fragment, newHTML)
		}
	}
	// 不再含失效引用
	if contentHasBrokenImage(newHTML, testBrokenDomain) {
		t.Fatalf("broken refs remain:\n%s", newHTML)
	}
}

// TestPlanInsufficientFails 源图不足：整篇失败且输入零改动（全有或全无）
func TestPlanInsufficientFails(t *testing.T) {
	content := `<img src="https://image.08rj.com/dead1.jpg"><img src="https://image.08rj.com/dead2.jpg">`
	onlyOneSource := []string{"https://cdn.new.com/new1.jpg"}

	if _, err := planContentReplacement(content, testBrokenDomain, onlyOneSource); err == nil {
		t.Fatal("expected error when source images are insufficient")
	}
	if _, err := applyContentReplacements(content, testBrokenDomain, onlyOneSource); err == nil {
		t.Fatal("expected error when replacement urls are insufficient")
	}
	// 输入内容未被改写（字符串不可变 + 失败路径不返回改写结果）
	if contentHasBrokenImage(content, testBrokenDomain) != true {
		t.Fatal("input content should remain untouched on failure")
	}
	if !strings.Contains(content, "https://image.08rj.com/dead1.jpg") {
		t.Fatal("original content mutated")
	}
}

// TestApplyIdempotent 已替换内容重复应用映射为 no-op
func TestApplyIdempotent(t *testing.T) {
	content := `<img src="https://image.08rj.com/dead1.jpg">`
	sourceImages := []string{"https://cdn.new.com/new1.jpg"}

	first, err := applyContentReplacements(content, testBrokenDomain, sourceImages)
	if err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	// 再次 plan：已无失效 img → 计划为空且无错误
	plan, err := planContentReplacement(first, testBrokenDomain, sourceImages)
	if err != nil {
		t.Fatalf("second plan failed: %v", err)
	}
	if len(plan) != 0 {
		t.Fatalf("expected empty plan, got %v", plan)
	}
	// 再次 apply：无改动
	second, err := applyContentReplacements(first, testBrokenDomain, nil)
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if second != first {
		t.Fatalf("re-apply should be a no-op:\nfirst:  %s\nsecond: %s", first, second)
	}
}

// ---- inspectRepairNeeds / contentHasAnyImage / prependImage / 标记 ----

// TestInspectRepairNeeds 修复需求判定（与仓储候选条件一致）
func TestInspectRepairNeeds(t *testing.T) {
	cases := []struct {
		name      string
		thumbnail string
		content   string
		expected  repairNeeds
	}{
		{
			name:      "完全正常：不修复",
			thumbnail: "https://img.08rj.com/a.jpg",
			content:   `<img src="https://img.08rj.com/b.jpg">`,
			expected:  repairNeeds{},
		},
		{
			name:      "缺缩略图",
			thumbnail: "",
			content:   `<img src="https://img.08rj.com/b.jpg">`,
			expected:  repairNeeds{missingThumb: true},
		},
		{
			name:      "正文无图",
			thumbnail: "https://img.08rj.com/a.jpg",
			content:   `<p>纯文字内容</p>`,
			expected:  repairNeeds{missingContentImg: true},
		},
		{
			name:      "缩略图与正文均缺",
			thumbnail: "",
			content:   `<p>纯文字内容</p>`,
			expected:  repairNeeds{missingThumb: true, missingContentImg: true},
		},
		{
			name:      "缩略图引用失效域名",
			thumbnail: "https://image.08rj.com/dead.jpg",
			content:   `<img src="https://img.08rj.com/b.jpg">`,
			expected:  repairNeeds{brokenThumb: true},
		},
		{
			name:      "正文 img 引用失效域名",
			thumbnail: "https://img.08rj.com/a.jpg",
			content:   `<img src="https://image.08rj.com/dead.jpg">`,
			expected:  repairNeeds{brokenContent: true},
		},
		{
			name:      "正文文本提及失效域名不算引用",
			thumbnail: "https://img.08rj.com/a.jpg",
			content:   `<p>原图床 image.08rj.com 已关闭</p><img src="https://img.08rj.com/b.jpg">`,
			expected:  repairNeeds{},
		},
		{
			name:      "空白缩略图视为缺失",
			thumbnail: "   ",
			content:   `<img src="https://img.08rj.com/b.jpg">`,
			expected:  repairNeeds{missingThumb: true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			item := articleFixture{thumbnail: c.thumbnail, content: c.content}.toArticle()
			got := inspectRepairNeeds(item, testBrokenDomain)
			if got != c.expected {
				t.Fatalf("inspectRepairNeeds = %+v, want %+v", got, c.expected)
			}
		})
	}
}

// TestRepairNeedsActions 需求动作摘要
func TestRepairNeedsActions(t *testing.T) {
	if got := (repairNeeds{missingThumb: true, missingContentImg: true}).actions(); got != "thumb:restored,content:prepend" {
		t.Fatalf("unexpected actions: %q", got)
	}
	if got := (repairNeeds{brokenContent: true}).actions(); got != "content:replaced" {
		t.Fatalf("unexpected actions: %q", got)
	}
}

// TestContentHasAnyImage 正文是否含任意图片
func TestContentHasAnyImage(t *testing.T) {
	if contentHasAnyImage(`<p>没有图片</p>`) {
		t.Fatal("expected no image")
	}
	if !contentHasAnyImage(`<p>前文</p><img src="/upload/x.jpg">`) {
		t.Fatal("expected image found (relative src counts)")
	}
	if contentHasAnyImage(`<img src="">`) {
		t.Fatal("empty src should not count")
	}
}

// TestPrependImage 图片插入正文最前面：原正文完整保留在后
func TestPrependImage(t *testing.T) {
	original := `<p>正文第一段</p><p>第二段</p>`
	got := prependImage(original, "https://img.08rj.com/20260902/x.png")
	if !strings.HasPrefix(got, `<p><img src="https://img.08rj.com/20260902/x.png" alt=""></p>`) {
		t.Fatalf("image not at front:\n%s", got)
	}
	if !strings.HasSuffix(got, original) {
		t.Fatalf("original content not preserved:\n%s", got)
	}
}

// TestMarkImageRepairFailed 源页无图标记：写入一次、不重复写、可读取
func TestMarkImageRepairFailed(t *testing.T) {
	item := articleFixture{thumbnail: "", content: "<p>x</p>"}.toArticle()
	if item.Extends.Get(markerImageRepairFailed) != nil {
		t.Fatal("marker should not exist initially")
	}
	markImageRepairFailed(item)
	markImageRepairFailed(item) // 重复标记不追加
	if item.Extends.Get(markerImageRepairFailed) == nil {
		t.Fatal("marker missing after mark")
	}
	if len(item.Extends) != 1 {
		t.Fatalf("expected 1 extends item, got %d", len(item.Extends))
	}
}

// ---- 测试辅助 ----

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	return u
}

func newTestDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc
}

// articleFixture 构造最小可用的文章实体
type articleFixture struct{ thumbnail, content string }

func (f articleFixture) toArticle() *entity.Article {
	return &entity.Article{
		ArticleBase:   entity.ArticleBase{Thumbnail: f.thumbnail},
		ArticleDetail: entity.ArticleDetail{Content: f.content},
	}
}
