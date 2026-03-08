package baidu_utils

import (
	"fmt"
	"strings"
)

// GetRootDirList 获取根目录列表（只返回目录）
func (b *BaiduUtils) GetRootDirList() ([]BaiduDirItem, error) {
	items, err := b.GetDirList("/")
	if err != nil {
		return nil, err
	}

	// 只返回目录
	var dirs []BaiduDirItem
	for _, item := range items {
		if item.IsDir == 1 {
			dirs = append(dirs, item)
		}
	}

	return dirs, nil
}

// CreateDirectory 创建目录（自动检查是否已存在）
func (b *BaiduUtils) CreateDirectory(path string) error {
	// 检查目录是否已存在
	items, err := b.GetDirList("/")
	if err == nil {
		for _, item := range items {
			if item.IsDir == 1 && item.ServerFilename == path {
				// 目录已存在，直接返回
				return nil
			}
		}
	}

	// 创建目录
	fullPath := "/" + path
	if strings.HasPrefix(path, "/") {
		fullPath = path
	}

	return b.CreateDir(fullPath)
}

// GetFileInfo 获取文件信息
func (b *BaiduUtils) GetFileInfo(path string) (*BaiduDirItem, error) {
	// 获取父目录列表
	parentPath := "/"
	if idx := strings.LastIndex(path, "/"); idx > 0 {
		parentPath = path[:idx]
	}

	items, err := b.GetDirList(parentPath)
	if err != nil {
		return nil, err
	}

	// 查找指定文件
	filename := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		filename = path[idx+1:]
	}

	for _, item := range items {
		if item.ServerFilename == filename {
			return &item, nil
		}
	}

	return nil, fmt.Errorf("文件不存在: %s", path)
}

// EnsureDirectory 确保目录存在（自动创建）
func (b *BaiduUtils) EnsureDirectory(path string) error {
	// 如果路径为空，返回根目录
	if path == "" {
		return nil
	}

	// 标准化路径（去掉开头的 /）
	path = strings.TrimPrefix(path, "/")

	// 如果路径包含 /，需要创建多级目录
	if strings.Contains(path, "/") {
		parts := strings.Split(path, "/")
		currentPath := ""

		for _, part := range parts {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = currentPath + "/" + part
			}

			// 检查并创建目录
			if err := b.CreateDirectory(currentPath); err != nil {
				return fmt.Errorf("创建目录失败: %s, 错误: %w", currentPath, err)
			}
		}

		return nil
	}

	// 单级目录
	return b.CreateDirectory(path)
}

// GetDirectoryTree 获取目录树（递归）
func (b *BaiduUtils) GetDirectoryTree(path string) ([]BaiduDirItem, error) {
	items, err := b.GetDirList(path)
	if err != nil {
		return nil, err
	}

	var tree []BaiduDirItem
	for _, item := range items {
		tree = append(tree, item)

		// 如果是目录，递归获取子目录
		if item.IsDir == 1 {
			subPath := path
			if !strings.HasSuffix(path, "/") {
				subPath = path + "/"
			}
			subPath = subPath + item.ServerFilename

			subItems, err := b.GetDirectoryTree(subPath)
			if err == nil {
				tree = append(tree, subItems...)
			}
		}
	}

	return tree, nil
}