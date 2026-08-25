# BENZHI_README

## 项目说明
- 项目：benzhi-project-1b92c9b3-b8fb-44d5-a1dd-ba74a9d46d71
- 项目用途：文物展柜环境异常处置台提供异常登记、风险分级、任务分派、现场处置、复测复核和证据归档关闭的浏览器工作台。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：文物展柜环境异常处置台
- 项目概述：面向博物馆值班与文保团队的展柜环境异常处置工作台，将传感器告警转为可追踪任务，经过分派、现场处置、复核和证据归档后关闭异常。
- 核心工作流：展柜异常告警登记→风险分级→处置任务分派→现场措施记录→复测复核→主管批准→证据归档并关闭
- 对外接口：Go HTTP 服务提供原生 HTML、CSS 和 JavaScript 工作台，支持 -addr=127.0.0.1:<port> 或 PORT 环境变量，默认监听 127.0.0.1:19081，浏览器完成异常处置全流程

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server --smoke -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-1b92c9b3-b8fb-44d5-a1dd-ba74a9d46d71-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-1b92c9b3-b8fb-44d5-a1dd-ba74a9d46d71-arm64 linux/arm64
docker run -it benzhi-project-1b92c9b3-b8fb-44d5-a1dd-ba74a9d46d71-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server --smoke -addr=127.0.0.1:19081`
