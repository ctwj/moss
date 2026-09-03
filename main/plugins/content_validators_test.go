package plugins

import "testing"

// TestIsPlaceholderText 占位/无效文本判定（表驱动）
func TestIsPlaceholderText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool // true = 是占位文本（无效）
	}{
		{"半角点号", "...", true},
		{"双点", "..", true},
		{"省略号", "…", true},
		{"中文句号", "。。。", true},
		{"点号带空格", " . . . ", true},
		{"省略号带点", "……...", true},
		{"横线", "---", true},
		{"问号", "???", true},
		{"波浪线", "~~~", true},
		{"全角点", "．．．", true},
		{"纯空白", "   \t\n ", true},
		{"空字符串", "", true},
		{"混合符号", ".-~!?。！？…—", true},
		{"正常中文", "微信WeChat电脑版", false},
		{"正常英文", "Notepad++ v8.6", false},
		{"字母数字混合", "abc123", false},
		{"单个汉字", "好", false},
		{"符号夹字母", "...a...", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsPlaceholderText(c.in); got != c.want {
				t.Errorf("IsPlaceholderText(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestCountAlphanumericHan 可读字符计数
func TestCountAlphanumericHan(t *testing.T) {
	if got := CountAlphanumericHan("微信电脑版下载"); got != 7 {
		t.Errorf("中文计数 = %d, want 7", got)
	}
	if got := CountAlphanumericHan("WeChat"); got != 6 {
		t.Errorf("英文计数 = %d, want 6", got)
	}
	if got := CountAlphanumericHan("v4.1"); got != 3 { // v 4 1
		t.Errorf("版本号计数 = %d, want 3", got)
	}
	if got := CountAlphanumericHan("..."); got != 0 {
		t.Errorf("符号计数 = %d, want 0", got)
	}
	if got := CountAlphanumericHan(""); got != 0 {
		t.Errorf("空串计数 = %d, want 0", got)
	}
}

// TestIsValidArticleTitle 标题有效性阈值判定
func TestIsValidArticleTitle(t *testing.T) {
	cases := []struct {
		name string
		in   string
		min  int
		want bool
	}{
		{"默认阈值_正常标题", "微信WeChat电脑版下载", 0, true},
		{"默认阈值_占位", "...", 0, false},
		{"默认阈值_过短3字", "abc", 0, false},
		{"默认阈值_恰好4字", "abcd", 0, true},
		{"自定义阈值2_3字通过", "abc", 2, true},
		{"自定义阈值10_不足", "abcd", 10, false},
		{"阈值0回落默认", "ab", 0, false}, // min<=0 → 默认4，2<4 无效
		{"空白包围的有效标题", "  好用软件  ", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got bool
			if c.min > 0 {
				got = IsValidArticleTitleWithMin(c.in, c.min)
			} else {
				got = IsValidArticleTitle(c.in)
			}
			if got != c.want {
				t.Errorf("IsValidArticleTitle(%q, min=%d) = %v, want %v", c.in, c.min, got, c.want)
			}
		})
	}
}

// TestIsValidArticleContent 正文有效性阈值判定（剥 HTML）
func TestIsValidArticleContent(t *testing.T) {
	long := "<p>" + repeat("软件下载站提供绿色软件下载服务。", 3) + "</p>" // 3*13=39 可读字符
	short := "<p>只有一句话。</p>"                                          // 6 可读字符
	placeholder := "<p>...</p>"
	nested := "<div><span>软件</span><img src=\"x.png\"><b>下载</b></div>" // 4 可读字符

	cases := []struct {
		name string
		in   string
		min  int
		want bool
	}{
		{"默认阈值_足够正文", long, 0, true},
		{"默认阈值_过短", short, 0, false},
		{"默认阈值_占位正文", placeholder, 0, false},
		{"阈值5_短文恰通过", short, 5, true}, // 「只有一句话」恰 5 个可读字符
		{"阈值6_短文不足", short, 6, false},
		{"阈值7_短文不足", short, 7, false},
		{"标签不计入可读字符", nested, 10, false}, // 仅 4 可读字符
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got bool
			if c.min > 0 {
				got = IsValidArticleContentWithMin(c.in, c.min)
			} else {
				got = IsValidArticleContent(c.in)
			}
			if got != c.want {
				t.Errorf("IsValidArticleContent(len=%d, min=%d) = %v, want %v", len([]rune(c.in)), c.min, got, c.want)
			}
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
