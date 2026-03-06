package plugins

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"moss/domain/config"
	"moss/domain/core/entity"
	"moss/domain/core/service"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/persistent/storage"
	"moss/infrastructure/support/upload"
	"moss/infrastructure/utils/imagex"
	"moss/infrastructure/utils/request"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bitly/go-simplejson"
	"github.com/duke-git/lancet/v2/cryptor"
	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
	"go.uber.org/zap"
)

type SaveArticleImages struct {
	EnableOnCreate bool `json:"enable_on_create"` // 创建时执行
	EnableOnUpdate bool `json:"enable_on_update"` // 更新时执行

	MaxWidth          int    `json:"max_width"`           // 最大图片宽度(像素)，大于此宽度将被等比例缩放
	MaxHeight         int    `json:"max_height"`          // 最大图片高度(像素)，大于此高度将被等比例缩放
	ThumbWidth        int    `json:"thumb_width"`         // 缩略图宽度(像素)
	ThumbHeight       int    `json:"thumb_height"`        // 缩略图高度(像素)
	ThumbMinWidth     int    `json:"thumb_min_width"`     // 选取缩略图时，限制最小缩略图宽度(像素)，小于此宽度的图片不会被选取成缩略图
	ThumbMinHeight    int    `json:"thumb_min_height"`    // 选取缩略图时，限制最小缩略图高度(像素)，小于此高度的图片不会被选取成缩略图
	AlwaysResize      bool   `json:"always_resize"`       // 是否始终缩放一下图片，已减少图片体积
	ThumbExtractFocus bool   `json:"thumb_extract_focus"` // 生成缩略图是提取焦点方式生成
	RemoveIfDownFail  bool   `json:"remove_if_down_fail"` // 下载失败是否删除
	DownRetry         int    `json:"down_retry"`          // 重试次数
	DownReferer       string `json:"down_referer"`        // 下载referer
	DownProxy         string `json:"down_proxy"`          // 下载代理
	UploadTarget      string `json:"upload_target"`       // 上传目标: local/api
	APIUploadURL      string `json:"api_upload_url"`      // 图床API地址
	APIFileField      string `json:"api_file_field"`      // 图床文件字段名
	APIHeaders        string `json:"api_headers"`         // 图床请求头(每行 key: value)
	APIFormData       string `json:"api_form_data"`       // 图床附加表单(每行 key=value)
	APIURLPath        string `json:"api_url_path"`        // 图床返回图片URL路径(如 data.url)
	APISuccessPath    string `json:"api_success_path"`    // 图床返回成功标识路径(可选)
	APISuccessValue   string `json:"api_success_value"`   // 图床返回成功标识值
	APITimeout        int    `json:"api_timeout"`         // 图床上传超时(秒)
	APIProxy          string `json:"api_proxy"`           // 图床上传代理
	APIImageDomain    string `json:"api_image_domain"`    // 图床图片域名(用于跳过重复上传)

	ctx         *pluginEntity.Plugin
	downReferer []saveArticleImagesDownReferer
}

func NewSaveArticleImages() *SaveArticleImages {
	return &SaveArticleImages{
		EnableOnCreate:    true,
		EnableOnUpdate:    true,
		DownRetry:         3,
		MaxWidth:          1000,
		MaxHeight:         2000,
		ThumbWidth:        230,
		ThumbHeight:       138,
		ThumbMinWidth:     100,
		ThumbMinHeight:    100,
		AlwaysResize:      true,
		ThumbExtractFocus: true,
		RemoveIfDownFail:  true,
		DownReferer:       "bdimg bdstatic http://www.baidu.com/\ntoutiaoimg http://www.toutiao.com/",
		UploadTarget:      "local",
		APIFileField:      "file",
		APIURLPath:        "data.url",
		APISuccessValue:   "true",
		APITimeout:        30,
	}
}

func (s *SaveArticleImages) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:    "SaveArticleImages",
		About: "save article images",
	}
}

func (s *SaveArticleImages) Run(ctx *pluginEntity.Plugin) error {
	return nil
}

func (s *SaveArticleImages) Load(ctx *pluginEntity.Plugin) error {
	s.ctx = ctx
	service.Article.AddCreateBeforeEvents(s)
	service.Article.AddUpdateBeforeEvents(s)
	return nil
}
func (s *SaveArticleImages) ArticleCreateBefore(item *entity.Article) (err error) {
	if !s.EnableOnCreate {
		return nil
	}
	return s.Save(item)
}
func (s *SaveArticleImages) ArticleUpdateBefore(item *entity.Article) (err error) {
	if !s.EnableOnUpdate {
		return nil
	}
	return s.Save(item)
}

func (s *SaveArticleImages) Save(item *entity.Article) error {
	s.initDownReferer()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(item.Content))
	if err != nil {
		s.ctx.Log.Error("format html document error", zap.Error(err), zap.String("title", item.Title))
		return err
	}
	doc.Find("img").Each(s.eachSave(item))
	s.saveThumbnail(item)
	html, err := doc.Find("body").Html()
	if err != nil {
		s.ctx.Log.Error("get html code error", zap.Error(err), zap.String("title", item.Title))
		return err
	}
	item.Content = html
	return nil
}

// 判断图片地址是否是当前定义的上传域
func (s *SaveArticleImages) isCurrentUploadDomain(imgURL string) bool {
	// upload域开头直接跳过
	if strings.HasPrefix(imgURL, config.Config.Upload.Domain) {
		return true
	}
	// 检测图片URL是否包含上传域名
	if uri, err := url.Parse(config.Config.Upload.Domain); err == nil {
		if uri.Host != "" && strings.Contains(imgURL, uri.Host) {
			return true
		}
	}
	// 额外支持API图床域名，防止反复上传
	if s.APIImageDomain != "" {
		if strings.HasPrefix(imgURL, s.APIImageDomain) {
			return true
		}
		if uri, err := url.Parse(s.APIImageDomain); err == nil {
			if uri.Host != "" && strings.Contains(imgURL, uri.Host) {
				return true
			}
		}
	}
	return false
}

func (s *SaveArticleImages) eachSave(item *entity.Article) func(i int, sn *goquery.Selection) {
	return func(i int, sn *goquery.Selection) {
		src, ok := sn.Attr("src")
		if !ok || src == "" {
			sn.Remove()
			return
		}
		if s.isCurrentUploadDomain(src) {
			return
		}
		if !strings.HasPrefix(src, "http") && !strings.HasPrefix(src, "//") { // 非远程图片
			return
		}
		if strings.HasPrefix(src, "data:") { // base64图片
			return
		}
		// 下载图片
		file, err := s.down(item, src)
		if err != nil && s.RemoveIfDownFail {
			sn.Remove()
			return
		}
		// 获取并判断图片类型
		imageType, err := filetype.Image(file)
		if imageType == types.Unknown || err != nil {
			s.ctx.Log.Warn("file is not a image type", s.logInfo(item, src, err)...)
			sn.Remove()
			return
		}
		// 获取图片尺寸
		size, _, err := image.DecodeConfig(bytes.NewReader(file))
		if size.Width == 0 || size.Height == 0 || err != nil {
			s.ctx.Log.Warn("image size error", s.logInfo(item, src, err)...)
			return
		}
		// 计算图片尺寸
		var width, height = imagex.ComputeScale(size.Width, size.Height, s.MaxWidth, s.MaxHeight)
		// 图片缩放，可以减少图片体积
		resized := false
		if s.AlwaysResize || size.Width > width || size.Height > height {
			if file, err = imagex.New().SetWidth(width).SetHeight(height).ResizeByte(file); err != nil {
				s.ctx.Log.Warn("image resize error", s.logInfo(item, src, err)...)
				return
			}
			// imagex.ResizeByte 当前输出为 jpeg
			imageType.Extension = ".jpg"
			imageType.MIME.Value = "image/jpeg"
			resized = true
		}
		// 上传图片
		hashSrc := cryptor.Md5String(src)
		uploadURL, err := s.uploadFile(hashSrc, imageType.Extension, imageType.MIME.Value, file)
		if err != nil {
			s.ctx.Log.Warn("upload image error", s.logInfo(item, src, err)...)
			return
		}
		s.ctx.Log.Info("upload image success", append(s.logInfo(item, src, nil), zap.String("url", uploadURL))...)
		// 设置标签属性
		sn.SetAttr("src", uploadURL)
		if resized {
			sn.SetAttr("width", strconv.Itoa(width))
			sn.SetAttr("height", strconv.Itoa(height))
		}

		// 上传缩略图
		if item.Thumbnail == "" && size.Width >= s.ThumbMinWidth && size.Height >= s.ThumbMinHeight {
			// 直接把内容中的图片保存成缩略图
			if err = s.uploadThumbnail(item, file, hashSrc+"_thumbnail", imageType.Extension, imageType.MIME.Value); err != nil {
				s.ctx.Log.Warn("upload thumbnail error", s.logInfo(item, src, err)...)
				return
			}
		}
	}
}

func (s *SaveArticleImages) logInfo(item *entity.Article, src string, err error) []zap.Field {
	return []zap.Field{zap.String("url", src), zap.String("title", item.Title), zap.Error(err)}
}

// 上传缩略图
func (s *SaveArticleImages) uploadThumbnail(item *entity.Article, file []byte, name, ext, imgType string) (err error) {
	rawFile := file
	if s.ThumbWidth > 0 || s.ThumbHeight > 0 {
		var imgLib = imagex.New().SetWidth(s.ThumbWidth).SetHeight(s.ThumbHeight)
		if s.ThumbExtractFocus {
			file, err = imgLib.CropByte(file)
		} else {
			file, err = imgLib.ThumbnailByte(file)
		}
		if err != nil {
			// 某些格式(如未注册解码器的webp)处理失败，回退原图上传，避免保留远程URL
			s.ctx.Log.Warn("thumbnail process failed, fallback to raw image", s.logInfo(item, item.Thumbnail, err)...)
			file = rawFile
		} else {
			// imagex 当前输出为 jpeg，上传元数据需同步
			ext = ".jpg"
			imgType = "image/jpeg"
		}
	}
	uploadURL, err := s.uploadFile(name, ext, imgType, file)
	if err != nil {
		return
	}
	s.ctx.Log.Info("upload thumbnail success", zap.String("title", item.Title), zap.String("url", uploadURL))
	item.Thumbnail = uploadURL
	return
}

func (s *SaveArticleImages) down(item *entity.Article, uri string) (file []byte, err error) {
	file, err = request.New().SetRetry(s.DownRetry).SetProxyURLStr(s.DownProxy).SetReferer(s.getDownReferer(uri)).GetBody(uri)
	if err != nil {
		s.ctx.Log.Warn("down file error", s.logInfo(item, uri, err)...)
	}
	return
}

func (s *SaveArticleImages) saveThumbnail(item *entity.Article) {
	if item.Thumbnail == "" {
		return
	}
	// 判断是否是当前的上传域
	if s.isCurrentUploadDomain(item.Thumbnail) {
		return
	}
	// 下载图片
	file, err := s.down(item, item.Thumbnail)
	if err != nil && s.RemoveIfDownFail {
		item.Thumbnail = ""
		return
	}
	// 获取并判断图片类型
	imageType, err := filetype.Image(file)
	if imageType == types.Unknown || err != nil {
		s.ctx.Log.Warn("thumbnail is not a image type", s.logInfo(item, item.Thumbnail, err)...)
		item.Thumbnail = ""
		return
	}
	if err = s.uploadThumbnail(item, file, cryptor.Md5String(item.Thumbnail)+"_thumbnail", imageType.Extension, imageType.MIME.Value); err != nil {
		s.ctx.Log.Warn("upload thumbnail error", s.logInfo(item, item.Thumbnail, err)...)
	}
}

type saveArticleImagesDownReferer struct {
	rule    string
	referer string
}

func (s *SaveArticleImages) initDownReferer() {
	s.downReferer = nil
	if s.DownReferer == "" {
		return
	}
	for _, line := range strings.Split(s.DownReferer, "\n") {
		arr := strings.Split(line, " ")
		arrLen := len(arr)
		if arrLen < 2 {
			continue
		}
		referer := arr[arrLen-1]
		newArr := arr[:arrLen-1]
		for _, rule := range newArr {
			s.downReferer = append(s.downReferer, saveArticleImagesDownReferer{rule: rule, referer: referer})
		}
	}
}

func (s *SaveArticleImages) getDownReferer(src string) string {
	for _, v := range s.downReferer {
		if strings.Contains(src, v.rule) {
			return v.referer
		}
	}

	// Fallback: use the image origin as referer for anti-hotlink sites.
	if u, err := url.Parse(src); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/"
	}

	return ""
}

func (s *SaveArticleImages) uploadFile(name, ext, imgType string, file []byte) (string, error) {
	if strings.EqualFold(strings.TrimSpace(s.UploadTarget), "api") {
		return s.uploadByAPI(name, ext, imgType, file)
	}
	return s.uploadByStorage(name, ext, imgType, file)
}

func (s *SaveArticleImages) uploadByStorage(name, ext, imgType string, file []byte) (string, error) {
	val := storage.NewSetValueBytes(file)
	val.ContentType = imgType
	uploadResult, err := upload.Upload(name, ext, val)
	if err != nil {
		return "", err
	}
	return uploadResult.URL, nil
}

func (s *SaveArticleImages) uploadByAPI(name, ext, _ string, file []byte) (string, error) {
	if strings.TrimSpace(s.APIUploadURL) == "" {
		return "", errors.New("api_upload_url is required")
	}

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	fileField := strings.TrimSpace(s.APIFileField)
	if fileField == "" {
		fileField = "file"
	}
	filePart, err := writer.CreateFormFile(fileField, name+ext)
	if err != nil {
		return "", err
	}
	if _, err = filePart.Write(file); err != nil {
		return "", err
	}
	for k, v := range s.parseLinesToKV(s.APIFormData, "=") {
		_ = writer.WriteField(k, v)
	}
	if err = writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", s.APIUploadURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "moss-save-article-images/1.0")
	for k, v := range s.parseLinesToKV(s.APIHeaders, ":") {
		req.Header.Set(k, v)
	}

	resp, err := s.apiHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("api upload status %d: %s", resp.StatusCode, string(respBody[:minInt(len(respBody), 180)]))
	}

	js, err := simplejson.NewJson(respBody)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.APISuccessPath) != "" {
		successVal := s.jsonPathString(js, s.APISuccessPath)
		if !strings.EqualFold(strings.TrimSpace(successVal), strings.TrimSpace(s.APISuccessValue)) {
			return "", fmt.Errorf("api upload success check failed, path=%s value=%s", s.APISuccessPath, successVal)
		}
	}

	urlPath := strings.TrimSpace(s.APIURLPath)
	if urlPath == "" {
		urlPath = "data.url"
	}
	imageURL := strings.TrimSpace(s.jsonPathString(js, urlPath))
	if imageURL == "" {
		return "", fmt.Errorf("api upload url not found at path=%s", urlPath)
	}
	if strings.HasPrefix(imageURL, "//") {
		return "https:" + imageURL, nil
	}
	return imageURL, nil
}

func (s *SaveArticleImages) apiHTTPClient() *http.Client {
	timeout := s.APITimeout
	if timeout <= 0 {
		timeout = 30
	}

	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	if s.APIProxy != "" {
		if proxyURL, err := url.Parse(s.APIProxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: transport,
	}
}

func (s *SaveArticleImages) parseLinesToKV(raw, sep string) map[string]string {
	res := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		arr := strings.SplitN(line, sep, 2)
		if len(arr) != 2 {
			continue
		}
		k := strings.TrimSpace(arr[0])
		v := strings.TrimSpace(arr[1])
		if k == "" {
			continue
		}
		res[k] = v
	}
	return res
}

func (s *SaveArticleImages) jsonPathString(js *simplejson.Json, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	node := js.GetPath(strings.Split(path, ".")...)
	if node == nil {
		return ""
	}
	val := node.Interface()
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
