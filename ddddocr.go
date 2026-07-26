// Package ddddocr 提供基于 onnxruntime 的验证码文字识别（OCR），
// 是 Python 项目 ddddocr(https://github.com/sml2h3/ddddocr) 的 Go 移植。
//
// 典型用法：
//
//	rec, err := ddddocr.New(ddddocr.Config{ModelPath: "common.onnx"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer rec.Close()
//
//	text, err := rec.Classification(img) // img 为 image.Image
//
// 注意：本包依赖 onnxruntime 的原生共享库（.dll/.so/.dylib）与 onnx 模型文件，
// 二者均由调用方提供（不随包分发）。共享库路径可通过 Config.OnnxRuntimeLibPath
// 指定，留空则自动探测（环境变量 > 可执行文件目录 > 当前工作目录）。
package ddddocr

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

// Config 描述创建 Recognizer 所需的配置。
type Config struct {
	// ModelPath 是 onnx 模型文件路径（必填），例如 "common.onnx"。
	ModelPath string
	// OnnxRuntimeLibPath 是 onnxruntime 原生共享库路径（可选）。
	// 留空时自动探测：环境变量 ONNXRUNTIME_SHARED_LIBRARY_PATH >
	// 可执行文件所在目录 > 当前工作目录。
	OnnxRuntimeLibPath string
}

// Recognizer 封装一个可复用的 OCR 会话。模型只需加载一次，
// 之后可重复识别，避免每次都重新加载数十 MB 的模型。
//
// Recognizer 非并发安全：多协程并发使用时请自行加锁，
// 或为每个协程创建独立的 Recognizer。
type Recognizer struct {
	session *ort.DynamicAdvancedSession
}

// New 初始化 onnxruntime 全局环境（幂等，若已初始化则跳过）并加载模型，
// 返回可复用的 Recognizer。
func New(cfg Config) (*Recognizer, error) {
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("ddddocr: Config.ModelPath 不能为空")
	}

	// onnxruntime 环境是进程级全局单例，只初始化一次。
	// 首个调用者决定共享库路径；后续调用者复用已加载的库。
	if !ort.IsInitialized() {
		libPath := cfg.OnnxRuntimeLibPath
		if libPath == "" {
			libPath = sharedLibPath()
		}
		ort.SetSharedLibraryPath(libPath)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("ddddocr: 初始化 onnxruntime 环境失败: %w", err)
		}
	}

	session, err := ort.NewDynamicAdvancedSession(
		cfg.ModelPath,
		[]string{"input1"}, // 输入节点名（见模型 metadata）
		[]string{"output"}, // 输出节点名
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("ddddocr: 创建会话失败: %w", err)
	}
	return &Recognizer{session: session}, nil
}

// Close 释放本 Recognizer 占用的会话资源。
//
// 注意：Close 不会销毁进程级的 onnxruntime 全局环境，
// 因为其它 Recognizer 可能仍在使用。若需在程序退出前显式清理全局环境，
// 调用 Shutdown()。
func (r *Recognizer) Close() error {
	if r.session == nil {
		return nil
	}
	err := r.session.Destroy()
	r.session = nil
	return err
}

// Shutdown 销毁 onnxruntime 全局环境。通常无需调用（进程退出会自动清理），
// 仅在需要显式释放时于程序退出前调用一次，且此前必须已关闭所有 Recognizer。
func Shutdown() error {
	if !ort.IsInitialized() {
		return nil
	}
	return ort.DestroyEnvironment()
}

// Classification 识别一张验证码图片，返回识别出的文本。
func (r *Recognizer) Classification(img image.Image) (string, error) {
	if r.session == nil {
		return "", fmt.Errorf("ddddocr: Recognizer 已关闭")
	}
	bounds := img.Bounds()
	if bounds.Dy() == 0 {
		return "", fmt.Errorf("ddddocr: 图片高度为 0")
	}

	// 与 Python 官方模型一致：按高度 64 等比缩放。
	width := int(float64(bounds.Dx()) * (64.0 / float64(bounds.Dy())))
	if width <= 0 {
		return "", fmt.Errorf("ddddocr: 计算出的目标宽度非法: %d", width)
	}

	// 缩放使用 Lanczos3，与 PIL 的 LANCZOS(a=3) 对齐。
	resized := resize.Resize(uint(width), 64, img, resize.Lanczos3)
	inputData := preprocess(resized, width)

	inputTensor, err := ort.NewTensor(ort.NewShape(1, 1, 64, int64(width)), inputData)
	if err != nil {
		return "", fmt.Errorf("ddddocr: 创建输入张量失败: %w", err)
	}
	defer inputTensor.Destroy()

	// 输出形状是动态的（[1, -1]），传 nil 让会话自动分配输出张量。
	outputs := []ort.Value{nil}
	if err := r.session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return "", fmt.Errorf("ddddocr: 推理失败: %w", err)
	}
	outputTensor, ok := outputs[0].(*ort.Tensor[int64])
	if !ok {
		return "", fmt.Errorf("ddddocr: 输出张量类型异常: %T", outputs[0])
	}
	defer outputTensor.Destroy()

	return decode(outputTensor.GetData()), nil
}

// preprocess 将缩放后的图片转成模型输入用的 float32 序列（按行优先展平）。
//
// 与 Python 官方模型的预处理保持一致：
//  1. 转灰度（PIL 的 'L' 通道，即 ITU-R 601 亮度加权，Go 用 color.GrayModel 等价实现）
//  2. 像素值 /255 归一到 [0,1]
//  3. 再做 (x-0.5)/0.5，映射到 [-1,1]
func preprocess(img image.Image, width int) []float32 {
	bounds := img.Bounds()
	data := make([]float32, 64*width)
	i := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < width; x++ {
			c := color.GrayModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.Gray)
			v := float32(c.Y) / 255.0
			data[i] = (v - 0.5) / 0.5
			i++
		}
	}
	return data
}

// decode 对模型输出的类别索引做 CTC 贪心解码：
// 折叠连续重复项、去掉空白符（索引 0），再映射到字符表。
func decode(indices []int64) string {
	var sb []rune
	last := int64(-1)
	for _, item := range indices {
		if item == last {
			continue
		}
		last = item
		if item != 0 && int(item) < len(charset) {
			sb = append(sb, []rune(charset[item])...)
		}
	}
	return string(sb)
}

// defaultSharedLibName 返回当前操作系统对应的 onnxruntime 共享库文件名。
func defaultSharedLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "onnxruntime.dylib"
	default:
		return "onnxruntime.so"
	}
}

// sharedLibPath 自动探测 onnxruntime 共享库路径。
// 优先级：1) 环境变量 ONNXRUNTIME_SHARED_LIBRARY_PATH
//  2. 可执行文件所在目录
//  3. 当前工作目录
//
// 找到后返回绝对路径。注意：在 Windows 下如果只传裸文件名，
// 会因 DLL 搜索顺序而加载 PATH 中的其它 onnxruntime.dll，
// 因此必须解析为绝对路径。
func sharedLibPath() string {
	if p := os.Getenv("ONNXRUNTIME_SHARED_LIBRARY_PATH"); p != "" {
		return p
	}
	name := defaultSharedLibName()
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}
	for _, d := range dirs {
		candidate := filepath.Join(d, name)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	return name
}
