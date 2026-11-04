# 文档站维护

本站使用 Jekyll 和 GitHub Pages，文档源码保存在仓库的 `docs/`。发布工作流是 `.github/workflows/jekyll-gh-pages.yml`，站点名称为 **guanlan-ops**。

## 首次启用

1. 将工作流和文档提交、推送到 GitHub 的默认分支（当前配置为 `main`）。
2. 打开仓库 **Settings → Pages → Build and deployment**，将 **Source** 设为 **GitHub Actions**。
3. 在 **Actions → Documentation → Run workflow** 手动运行一次；后续修改文档后推送会自动更新。
4. 成功后在 Pages 设置页或工作流的 `github-pages` 环境中打开站点链接。

如果仓库名为 `guanlan-ops`，项目站点地址通常为 `https://<用户名>.github.io/guanlan-ops/`。工作流会读取 Pages 的真实域名和路径，兼容仓库改名与自定义域名。仓库改名后文档站标题仍为 `guanlan-ops`；标题可在 `docs/_config.yml` 修改。

默认分支不是 `main` 时，同步修改工作流的 `push.branches`。GitHub Pages 环境若设置了部署分支限制，也需要允许新的默认分支。

## 添加文档

- 在 `docs/` 或 `docs/architecture/` 新增 Markdown，用一级标题声明文档名称。
- 在 `docs/_data/navigation.yml` 添加侧栏入口；导航 URL 使用生成后的 `.html` 地址。
- 正文链接继续使用相对 `.md` 路径，Jekyll 在发布时自动转成页面地址，GitHub 源码浏览也可正常跳转。
- 跨出 `docs/` 的源码或部署文件链接会在构建准备阶段转换为对应 GitHub 文件地址。
- Mermaid 图表由浏览器加载固定版本的渲染器；CDN 不可用或图表语法不兼容时保留源码。
- 历史方案、讨论稿放在首页的「设计与规划」中，避免与已实现功能混淆。

## 发布流程

推送到 `main` 且修改文档、站点准备脚本或发布工作流时，自动构建并发布。Pull Request 只构建验证，不发布线上站点。手动发布也仅允许从默认分支执行部署。

工作流使用临时 `_pages_source/` 目录，为现有 Markdown 补充 Jekyll 元数据，保持原始文档不被改写。输出目录为 `_site/`，这两个目录都不提交 Git。

`Build` 工作流执行 `make build`，`Documentation` 工作流发布静态文档站，两者分别显示运行结果。Pages 只承载文档，观澜服务端仍需自行部署。
