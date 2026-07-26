package ddddocr

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
	"testing"
)

// loadTestImage 读取 testdata 下的 base64 验证码图片。
func loadTestImage(t *testing.T) image.Image {
	t.Helper()
	raw, err := os.ReadFile("testdata/captcha.txt")
	if err != nil {
		t.Fatalf("读取测试图片失败: %v", err)
	}
	data := strings.Join(strings.Fields(string(raw)), "")
	dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(data))
	b, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("图片解码失败: %v", err)
	}
	return img
}

// TestClassification 端到端验证：加载模型识别内置样本，期望得到 "4js4"。
// 需要 common.onnx 与 onnxruntime 共享库在包目录（仓库根）下。
func TestClassification(t *testing.T) {
	if _, err := os.Stat("common.onnx"); err != nil {
		t.Skip("缺少 common.onnx，跳过端到端测试")
	}
	rec, err := New(Config{ModelPath: "common.onnx"})
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	defer rec.Close()

	got, err := rec.Classification(loadTestImage(t))
	if err != nil {
		t.Fatalf("Classification 失败: %v", err)
	}
	if want := "4js4"; got != want {
		t.Fatalf("识别结果 = %q, 期望 %q", got, want)
	}
}

// TestNewValidatesModelPath 校验空 ModelPath 会报错。
func TestNewValidatesModelPath(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("空 ModelPath 应返回错误")
	}
}

// TestCharsetLen 固化字符表长度，防止误改导致索引错位。
func TestCharsetLen(t *testing.T) {
	if got, want := len(charset), 8210; got != want {
		t.Fatalf("charset 长度 = %d, 期望 %d", got, want)
	}
}
