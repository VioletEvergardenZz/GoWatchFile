# 开发指南

## 环境准备
- Go 1.21+。
- S3 兼容对象存储（本地可用 MinIO）。
- Jenkins（当前版本为必填，用于触发处理动作）。
- 企业微信/钉钉机器人（可选）。

## 本地启动
```bash
cd go-watch-file
cp .env.example .env
# 根据注释填写 WATCH_DIR、FILE_EXT、S3、Jenkins 等配置

# 构建并运行
go build -o bin/file-watch cmd/main.go
./bin/file-watch -config config.yaml
```

## 配置要点
- `watch_dir` 必须存在且为目录。
- `file_ext` 仅支持单一后缀（如 `.log` / `.txt` / `.zip`）。
- 配置优先级：环境变量 > `.env` > `config.yaml` 占位符 > 默认值。

## 运行测试
```bash
cd go-watch-file
go test ./...
```

## 单机端到端验证（上传 + Jenkins + 通知）
1) 准备对象存储
- 本地 MinIO 示例：创建 Bucket，并记录 `S3_ENDPOINT`/`S3_BUCKET`。
- 若使用 MinIO：`S3_FORCE_PATH_STYLE=true`，必要时 `S3_DISABLE_SSL=true`。

2) 准备 Jenkins Job（最小可用）
- 建议创建 Pipeline Job，并设置参数：`DOWNLOAD_FILE`、`APP`、`FILE_NAME`。
- 示例 Jenkinsfile：
```groovy
pipeline {
  agent any
  parameters {
    string(name: 'DOWNLOAD_FILE', defaultValue: '')
    string(name: 'APP', defaultValue: '')
    string(name: 'FILE_NAME', defaultValue: '')
  }
  stages {
    stage('process') {
      steps {
        echo "DOWNLOAD_FILE=${params.DOWNLOAD_FILE}"
        echo "APP=${params.APP}"
        echo "FILE_NAME=${params.FILE_NAME}"
      }
    }
  }
}
```

3) 启动 Agent
- `WATCH_DIR` 指向本地测试目录（确保可写）。

4) 写入测试文件
```bash
# 在 watch_dir 下创建匹配后缀的文件
cp sample.log <watch_dir>/app-a/sample.log
```

5) 验证结果
- 日志出现“文件写入完成 / 上传成功 / 触发 Jenkins”。
- 对象存储中出现对应对象 Key。
- Jenkins 有一次构建记录。
- 若配置机器人，收到通知消息。

> Jenkins 目前为必填项；如需跳过触发，请使用可达的 Jenkins 或临时搭建一个最简 Pipeline 作为占位。
