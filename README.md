# ddddocr-for-golang

## 本项目提供golang版本的ddddocr简单demo，基于[sml2h3](https://github.com/sml2h3)开源的[ddddocr](https://github.com/sml2h3/ddddocr)构建。

1. 本项目提供base64转图片后识别验证码的功能
2. 用于学习golang的onnx模型调用，仅实现了ocr部分，支持ddddocr项目中列出的所有文字验证码，无opencv相关功能
3. 使用[onnxruntime_go](https://github.com/yalue/onnxruntime_go)调用[ddddocr](https://github.com/sml2h3/ddddocr)的onnx库
4. 使用[onnxruntime](https://github.com/microsoft/onnxruntime)最新版本的releases库即可，我使用的是[onnxruntime-win-x64-1.16.3.zip](https://github.com/microsoft/onnxruntime/releases/download/v1.16.3/onnxruntime-win-x64-1.16.3.zip)