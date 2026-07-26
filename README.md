# ddddocr-for-golang

Go 版本的验证码识别库，基于 [sml2h3](https://github.com/sml2h3) 开源的 [ddddocr](https://github.com/sml2h3/ddddocr) 模型构建，可作为包 `go get` 直接导入使用。

- 支持 ddddocr 通用模型的**全部文字验证码**：中文 / 英文 / 数字 / 混合 / 算术题等，无 opencv 依赖
- 通过 [yalue/onnxruntime_go](https://github.com/yalue/onnxruntime_go) 调用 onnxruntime 做推理
- **跨平台**：Windows / Linux / macOS
- 目前仅实现 OCR 文字识别；滑块 / 目标检测需另接 `common_det.onnx`（暂未集成）

## 依赖版本

| 组件 | 版本 |
| --- | --- |
| Go | 1.21+ |
| [yalue/onnxruntime_go](https://github.com/yalue/onnxruntime_go) | v1.31.0 |
| [onnxruntime](https://github.com/microsoft/onnxruntime) 运行时 | **1.28.0**（需 ≥ 1.26，因为 yalue v1.31.0 要求 ORT API 版本 26） |

> ⚠️ yalue 的 Go 绑定与 onnxruntime 运行时是**版本耦合**的：绑定会在初始化时请求某个 ORT API 版本，运行时必须支持该版本。
> 若要升级/降级其中之一，请对照 [yalue Releases](https://github.com/yalue/onnxruntime_go/releases) 同步调整另一个。

## 模型说明

使用 ddddocr 的 `common.onnx`（beta 通用模型）。`charset.go` 中的字符表（8210 项）与该模型**逐元素校验一致于当前 Python 版**，即模型已与上游对齐，无需更新。

> ⚠️ 模型与字符表是**严格配套的一对**：`common.onnx` 输出的是类别索引，靠 `charset.go` 翻译成字符。若升级模型，**必须同时**从 Python 源码同步字符表，否则会出现「能跑但全是乱码」。

## 跨平台运行

本项目已支持 Windows / Linux / macOS，代码会按操作系统自动选择共享库文件名并解析为绝对路径加载。

你只需为对应平台准备 onnxruntime 运行时库，从 [onnxruntime releases](https://github.com/microsoft/onnxruntime/releases/tag/v1.28.0) 下载并解压，把库文件放到**可执行文件同目录**或**当前工作目录**：

| 平台 | 需要的库文件 | 下载包 |
| --- | --- | --- |
| Windows x64 | `onnxruntime.dll` | `onnxruntime-win-x64-1.28.0.zip` |
| Linux x64 | `onnxruntime.so`（可由 `libonnxruntime.so.1.28.0` 重命名/软链） | `onnxruntime-linux-x64-1.28.0.tgz` |
| macOS (arm64/x64) | `onnxruntime.dylib`（可由 `libonnxruntime.1.28.0.dylib` 重命名/软链） | `onnxruntime-osx-*-1.28.0.tgz` |

> 仓库内已随附 Windows x64 的 `onnxruntime.dll`，clone 后在 Windows 上可直接运行。

### 共享库加载路径优先级

`sharedLibPath()` 按以下顺序查找运行时库：

1. 环境变量 `ONNXRUNTIME_SHARED_LIBRARY_PATH`（最高优先级，可指向任意绝对路径）
2. 可执行文件所在目录
3. 当前工作目录

在 Windows 上必须解析为**绝对路径**，否则裸文件名会因 DLL 搜索顺序而误加载 PATH 中的其它 `onnxruntime.dll`。

## 代码结构

本仓库是一个可导入的 Go 包 `github.com/FeilongTest/go-ddddocr`（包名 `ddddocr`）：

| 文件/目录 | 职责 |
| --- | --- |
| `ddddocr.go` | 核心库：`Config` / `New` / `Recognizer.Classification` / `Close`，预处理、CTC 解码、共享库路径探测 |
| `charset.go` | 字符表（与模型严格配套，共 8210 项） |
| `ddddocr_test.go` | 单测（端到端识别 + 参数校验 + 字符表长度固化） |
| `example/main.go` | 可运行的 demo：用内置 base64 图片验证 |
| `common.onnx` / `onnxruntime.dll` | 模型与运行时库（仅供 example/测试使用，不随包分发） |

## 作为库使用

```bash
go get github.com/FeilongTest/go-ddddocr
```

```go
import ddddocr "github.com/FeilongTest/go-ddddocr"

// ModelPath 必填；OnnxRuntimeLibPath 留空则自动探测
rec, err := ddddocr.New(ddddocr.Config{ModelPath: "common.onnx"})
if err != nil {
    log.Fatal(err)
}
defer rec.Close()

text, err := rec.Classification(img) // img 为 image.Image
```

设计要点：

- **模型与运行时库由调用方提供**：`common.onnx` 与 onnxruntime 共享库不内嵌进包，通过 `Config` 指定路径（共享库留空则按 环境变量 > 可执行文件目录 > 当前目录 自动探测）。
- **全局环境幂等初始化**：`New` 内部用 `IsInitialized()` 守卫，多个 `Recognizer` 共享同一个进程级 onnxruntime 环境；`Close` 只释放自己的会话，不销毁全局环境。需显式清理时在退出前调用 `ddddocr.Shutdown()`。
- **非并发安全**：单个 `Recognizer` 不可多协程并发使用，高并发场景请加锁或每协程一个实例。

## 识别效果

取自上游/社区仓库的真实验证码实测（人眼真值 vs 本库输出）：

| 类型 | 真值 | 本库输出 | 结果 |
| --- | --- | --- | --- |
| 中文 | 极速换新 | 极速换新 | ✅ |
| 单字 | 酥 | 酥 | ✅ |
| 算术题 | 九乘六等于？ | 九乘六等于？ | ✅ |
| 字母数字 | 8A62N1 | 8A62N1 | ✅ |
| 内置样本 | 4js4 | 4js4 | ✅ |

中文、单字、算术、字母数字均可正确识别。滑块类图片（拼图块/背景图）非文字验证码，OCR 返回空或噪声属正常，需另接目标检测模型。

## 本次相较早期版本的升级点

1. **跨平台**：按 OS 自动选择 `.dll`/`.so`/`.dylib` 并解析为绝对路径，不再硬编码 Windows 绝对路径。
2. **升级依赖**：yalue `v1.5.0 → v1.31.0`，onnxruntime `1.16.3 → 1.28.0`。
3. **消除脆弱的重试 hack**：原来靠解析报错字符串猜测输出形状（`goto Retry`），改用 `DynamicAdvancedSession`，动态输出自动分配。
4. **修正预处理**：补齐与 Python 官方模型一致的归一化 `(x/255 - 0.5) / 0.5`（映射到 `[-1,1]`），灰度改用 ITU-R 601 亮度加权（对齐 PIL `convert('L')`），缩放改用 Lanczos3（对齐 PIL `LANCZOS`）。早期只做 `/255` 在复杂验证码上会掉准确率。
5. **可复用**：模型加载一次即可重复识别（`Recognizer`），不再每次识别都重建会话。
6. **包化**：从 `package main` 的学习 demo 改造成可 `go get` 导入的 `package ddddocr`，demo 移入 `example/`。

## 运行 demo

```bash
# 确保对应平台的 onnxruntime 运行时库与 common.onnx 在当前目录（或用环境变量指定库路径）
go run ./example
```

## 运行测试

```bash
go test ./...
```
