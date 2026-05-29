# 网文脱水机（Go 重构版）

AI 驱动的中文网文压缩工具，调用 DeepSeek API 将长篇网文浓缩为精华摘要。保留关键剧情、人物和伏笔，剔除注水内容，让你用 1/5 的时间读完整本书。

> 本项目重构复刻自 [SilenceX9/novel-dehydrator](https://github.com/SilenceX9/novel-dehydrator)，使用 Go 重新实现。相比 Python 原版镜像体积缩小 91%，内存占用降低 94%。

---

## 功能特性

- **批量上传**：支持 TXT / EPUB 格式，自动解析章节，断点续传
- **AI 脱水**：每章输出剧情摘要、关键人物、伏笔提示三段式结构
- **书单管理**：书籍分组归档，支持多维度排序
- **实时进度**：SSE 推送处理进度，无需刷新页面
- **并发处理**：可配置 goroutine 并发数，千章长文也能快速处理
- **导出下载**：处理完成后导出为 TXT 文件
- **密码保护**：可选的访问密码，适合公网部署
- **兼容 OpenAI 格式**：默认使用 DeepSeek，支持任何 OpenAI 兼容接口

---

## 快速开始

### 前置条件

- Go 1.22+
- PostgreSQL 14+（或 Docker）
- [DeepSeek API Key](https://platform.deepseek.com/)

### 构建与运行

```bash
git clone https://github.com/LogicShao/novel-dehydrator.git
cd novel-dehydrator

# 编译二进制
go build -o dehydrator ./cmd/server

# 配置环境变量
cp .env.example .env
# 编辑 .env，填入 DATABASE_URL 和 DEEPSEEK_API_KEY

# 创建数据库
createdb novel_dehydrator

# 启动服务，启动时会自动执行数据库迁移
./dehydrator
```

浏览器访问 [http://127.0.0.1:8765](http://127.0.0.1:8765)

---

## Docker 部署

### 单独构建镜像

```bash
docker build -t novel-dehydrator .

docker run -d \
  -p 8765:8765 \
  -e DATABASE_URL="postgres://user:pass@host:5432/novel_dehydrator?sslmode=disable" \
  -e DEEPSEEK_API_KEY="your_api_key" \
  -e DATA_DIR="/data" \
  -v $(pwd)/data:/data \
  novel-dehydrator
```

### docker-compose（含 PostgreSQL）

```bash
cp .env.example .env   # 填入 DEEPSEEK_API_KEY

docker compose up -d

# 查看日志
docker compose logs -f app
```

服务启动时会自动执行数据库迁移。镜像基于 scratch 多阶段构建，体积约 **13MB**。

---

## 环境变量

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `DATABASE_URL` | `postgres://localhost:5432/novel_dehydrator` | PostgreSQL 连接字符串 |
| `DEEPSEEK_API_KEY` | 必填 | DeepSeek API 密钥 |
| `DEEPSEEK_MODEL` | `deepseek-v4-flash` | 模型名称 |
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com` | API 地址，支持 OpenAI 兼容接口 |
| `AUTH_PASSWORD` | 空（不启用） | 访问密码保护 |
| `DEHYDRATE_CONCURRENCY` | `20` | 并发处理章节数（1-20） |
| `MAX_RETRIES` | `3` | 单章最大重试次数 |
| `CHUNK_CHAR_LIMIT` | `12000` | 单次 AI 请求最大字符数 |
| `PORT` | `8765` | 服务监听端口 |
| `DATA_DIR` | `data` | 上传文件和输出目录 |

---

## 项目结构

```
novel-dehydrator/
├── cmd/
│   ├── server/             # 服务入口
│   └── migrate/            # 迁移工具入口
├── internal/
│   ├── config/             # 环境变量配置
│   ├── db/                 # PostgreSQL 连接池
│   ├── handlers/           # chi 路由处理器
│   ├── middleware/         # 认证、日志中间件
│   ├── migration/          # 数据库迁移逻辑
│   ├── models/             # 数据模型
│   ├── router/             # 路由注册
│   ├── services/           # 业务逻辑（脱水、解析、导出）
│   ├── storage/            # 文件系统 I/O
│   └── logger/             # 日志封装
├── migrations/             # SQL 迁移文件
├── scripts/                # 辅助脚本
├── static/                 # CSS 和静态资源
├── templates/              # HTML 模板（Alpine.js）
├── Makefile
├── Dockerfile              # 多阶段 scratch 构建
└── docker-compose.yml
```

---

## 技术栈

| 层级 | 技术 |
|---|---|
| 后端 | Go + chi 路由 + pgx |
| 数据库 | PostgreSQL |
| AI | DeepSeek API（OpenAI 兼容） |
| 前端 | Alpine.js + HTML/CSS + SSE |
| 部署 | Docker 多阶段构建，scratch 基础镜像 |

---

## 与 Python 版对比

| 指标 | Go 版 | Python 版 |
|---|---|---|
| Docker 镜像体积 | ~13MB | ~150MB |
| 空载内存占用 | ~7.5MB | ~130MB |
| 启动时间 | <100ms | ~2s |
| 单文件二进制 | 支持 | 不支持 |

---

## 常见问题

**支持其他 AI 提供商吗？**
支持。任何 OpenAI 兼容的 Chat API 都可以用，修改 `DEEPSEEK_BASE_URL` 和 `DEEPSEEK_MODEL` 即可。

**并发数设多少合适？**
取决于 API 速率限制。DeepSeek 免费版建议 1-3，付费版可设 10-20。

**处理中断了怎么办？**
重新启动脱水任务，已完成的章节会自动跳过。

---

## 致谢

感谢 [SilenceX9/novel-dehydrator](https://github.com/SilenceX9/novel-dehydrator) 原始项目提供的产品思路与实现参考。

感谢 [Linux Do 社区](https://linux.do/) 的灵感来源与公益服务支持。

## License

MIT
