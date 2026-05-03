# Project Rules

## Tech Stack
- Go 1.22+
- Gin
- GORM v2
- React 18
- Vite
- Semi UI

## Architecture
- 必须遵守 Router -> Controller -> Service -> Model
- 优先在 middleware / controller / service / model / web 扩展
- 尽量不改 relay / provider adapter

## Database Red Lines
- 不允许 rename/drop 现有表字段
- 只允许新增表、补字段、补索引、映射表
- 兼容 SQLite / MySQL / PostgreSQL

## Financial Rules
- 所有资金变动必须落 ledger
- 历史价格必须快照化
- 支付回调必须幂等

## Output Format
每次回复都必须给：
1. 影响范围
2. 风险点
3. 最小改动方案
4. 自测步骤
5. 回滚方案