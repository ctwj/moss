package service

import (
	"regexp"
	"sort"
	"testing"
)

// mockTag 模拟标签结构
type mockTag struct {
	Name string
	Slug string
}

// TestReplaceTagLinksCore 测试核心替换逻辑
func TestReplaceTagLinksCore(t *testing.T) {
	// 模拟标签数据
	mockTags := []mockTag{
		{Name: "免费", Slug: "free"},
	}

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "简单文本替换",
			content:  "这是一个免费软件",
			expected: `这是一个<a href="/tag/free">免费</a>软件`,
		},
		{
			name:     "已有链接不重复替换",
			content:  `<a href="/tag/free">免费</a>版本`,
			expected: `<a href="/tag/free">免费</a>版本`,
		},
		{
			name:     "已有链接内容中不替换",
			content:  `<a href="/tag/free">免费</a>版，用户可以免费使用`,
			expected: `<a href="/tag/free">免费</a>版，用户可以<a href="/tag/free">免费</a>使用`,
		},
		{
			name:     "属性值中不替换",
			content:  `<a href="/tag/免费" title="免费下载">下载</a>`,
			expected: `<a href="/tag/免费" title="免费下载">下载</a>`,
		},
		{
			name:     "复杂嵌套场景",
			content:  `<p><a href="/tag/free">免费</a>使用：软件提供<a href="/tag/free">免费</a>版本，用户可以免费使用全部功能。</p>`,
			expected: `<p><a href="/tag/free">免费</a>使用：软件提供<a href="/tag/free">免费</a>版本，用户可以<a href="/tag/free">免费</a>使用全部功能。</p>`,
		},
		{
			name:     "用户实际问题场景-已有链接不应再被处理",
			content:  `<p><a href="/tag/free">免费</a>使用：软件提供免费版本，用户可以免费使用全部功能。</p>`,
			expected: `<p><a href="/tag/free">免费</a>使用：软件提供<a href="/tag/free">免费</a>版本，用户可以<a href="/tag/free">免费</a>使用全部功能。</p>`,
		},
		{
			name:     "多个连续的标签词",
			content:  `免费免费使用`,
			expected: `<a href="/tag/free">免费</a><a href="/tag/free">免费</a>使用`,
		},
		{
			name:     "图片alt属性中不应替换",
			content:  `<img alt="免费软件" src="test.jpg"/>`,
			expected: `<img alt="免费软件" src="test.jpg"/>`,
		},
		{
			name:     "复杂HTML结构",
			content:  `<p>这是一个免费软件</p><img alt="免费下载" src="test.jpg"/><p>免费使用</p>`,
			expected: `<p>这是一个<a href="/tag/free">免费</a>软件</p><img alt="免费下载" src="test.jpg"/><p><a href="/tag/free">免费</a>使用</p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceTagLinksMock(tt.content, mockTags)
			if result != tt.expected {
				t.Errorf("结果不匹配:\n期望: %s\n实际: %s", tt.expected, result)
			}
		})
	}
}

// replaceTagLinksMock 模拟 replaceTagLinks 的核心逻辑
func replaceTagLinksMock(content string, tags []mockTag) string {
	if content == "" {
		return content
	}

	// 找出所有HTML标签的位置
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	htmlMatches := tagPattern.FindAllStringIndex(content, -1)

	// 找出所有 <a> 标签的范围（包括其内容）
	aTagPattern := regexp.MustCompile(`<a\s[^>]*>.*?</a>`)
	aTagMatches := aTagPattern.FindAllStringIndex(content, -1)

	// 构建一个函数检查某个位置是否在HTML标签内部（属性值等）
	isInHTMLTagAttr := func(pos, length int) bool {
		end := pos + length
		for _, m := range htmlMatches {
			if pos >= m[0] && end <= m[1] {
				return true
			}
		}
		return false
	}

	// 构建一个函数检查某个位置是否在 <a> 标签内容中（不应再创建链接）
	isInATagContent := func(pos, length int) bool {
		end := pos + length
		for _, m := range aTagMatches {
			if pos >= m[0] && end <= m[1] {
				return true
			}
		}
		return false
	}

	// 收集所有需要替换的位置 {start, end, link}
	type replaceItem struct {
		start int
		end   int
		link  string
	}
	var allReplacements []replaceItem

	// 对每个标签名收集替换位置
	for _, tag := range tags {
		if tag.Name == "" {
			continue
		}
		tagName := tag.Name
		link := `<a href="/tag/` + tag.Slug + `">` + tagName + `</a>`

		// 找出该标签名在内容中的所有位置
		for i := 0; i <= len(content)-len(tagName); i++ {
			if content[i:i+len(tagName)] == tagName {
				// 检查是否是独立的单词（前后不是字母数字）
				beforeOK := i == 0 || !isAlphaNum(rune(content[i-1]))
				afterOK := i+len(tagName) >= len(content) || !isAlphaNum(rune(content[i+len(tagName)]))
				// 不在HTML标签属性中，也不在已有 <a> 标签内容中
				if beforeOK && afterOK && !isInHTMLTagAttr(i, len(tagName)) && !isInATagContent(i, len(tagName)) {
					allReplacements = append(allReplacements, replaceItem{
						start: i,
						end:   i + len(tagName),
						link:  link,
					})
				}
			}
		}
	}

	// 如果没有需要替换的，直接返回
	if len(allReplacements) == 0 {
		return content
	}

	// 去重：同一位置只替换一次
	seen := make(map[int]bool)
	var uniqueReplacements []replaceItem
	for _, r := range allReplacements {
		if !seen[r.start] {
			seen[r.start] = true
			uniqueReplacements = append(uniqueReplacements, r)
		}
	}

	// 按位置从后向前排序
	sort.Slice(uniqueReplacements, func(i, j int) bool {
		return uniqueReplacements[i].start > uniqueReplacements[j].start
	})

	// 从后向前替换
	result := content
	for _, r := range uniqueReplacements {
		result = result[:r.start] + r.link + result[r.end:]
	}

	return result
}
