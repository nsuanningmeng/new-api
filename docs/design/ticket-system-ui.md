# UI/UX Design: Ticket System

**Date**: 2026-05-03
**Target Platform**: Web (Desktop-first, mobile-compatible)
**Tech Stack**: React 18 + Semi UI + react-router-dom + react-i18next

---

## 1. Design Goals

### 1.1 User Goals

- End users: fast ticket creation, clear visibility of their own tickets' progress
- Resellers / Admins: efficiently triage, reply, and close tickets within their scope
- All roles: intuitive timeline-based conversation flow, unambiguous status distinction

### 1.2 Business Goals

- Reduce support response time by centralizing communication
- Maintain clear audit trail (who said what, when)
- Enforce multi-tenant permission isolation (each role only sees scoped tickets)

---

## 2. Page Structure

### 2.1 Route Layout

```
/console/ticket          -> TicketListPage   (PrivateRoute)
/console/ticket/:id      -> TicketDetailPage (PrivateRoute)
```

Both routes wrapped by `<PrivateRoute>` (not `<AdminRoute>`) since end users also need access. Scope filtering is handled server-side by API.

### 2.2 Sidebar Integration

Add `ticket` to the `console` section in `SiderBar.jsx`:

```
workspaceItems = [
  ...existing items...,
  {
    text: t('工单'),
    itemKey: 'ticket',
    to: '/ticket',
  },
]
```

Corresponding additions:

| File | Change |
|------|--------|
| `SiderBar.jsx` > `routerMap` | `ticket: '/console/ticket'` |
| `SiderBar.jsx` > `workspaceItems` | Add `{ text: t('工单'), itemKey: 'ticket', to: '/ticket' }` |
| `render.jsx` > `getLucideIcon` | Add `case 'ticket': return <TicketCheck {...commonProps} color={iconColor} />;` (from `lucide-react`) |
| `useSidebar.js` > `DEFAULT_ADMIN_CONFIG` | Add `ticket: true` to `console` section |
| `App.jsx` | Add `<Route path='/console/ticket' element={<PrivateRoute><Ticket /></PrivateRoute>} />` and `<Route path='/console/ticket/:id' element={<PrivateRoute><TicketDetail /></PrivateRoute>} />` |

### 2.3 Ticket List Page Layout

```
+-------------------------------------------------------+
| CardPro (type="type1")                                |
|                                                       |
|  [DescriptionArea]                                    |
|    "工单管理"  说明文字                                |
|  ─────────────────────────────────────────────        |
|  [ActionsArea]                                        |
|    [+ 创建工单]          [状态: All/Open/...]  [搜索] |
|  ─────────────────────────────────────────────        |
|  [Table]                                              |
|    | 标题 | 状态 | 优先级 | 创建者 | 更新时间 | 操作 ||
|    | ...  | Tag  | Tag   | text   | time    | 查看  ||
|    | ...  | ...  | ...   | ...    | ...     | ...   ||
|  ─────────────────────────────────────────────        |
|  [PaginationArea]                                     |
|    < 1 2 3 ... >                      10条/页         |
+-------------------------------------------------------+
```

### 2.4 Ticket Detail Page Layout

```
+-------------------------------------------------------+
| [< 返回列表]                                          |
|                                                       |
| Card: Ticket Header                                   |
|  标题: xxxxxxxx                                       |
|  状态: [Tag:open]  优先级: [Tag:普通]                  |
|  创建者: username   创建时间: 2026-05-03 10:00        |
|  ───────────────────────────────────                  |
|  [关闭工单] (admin/reseller only, shown when open)    |
+-------------------------------------------------------+
|                                                       |
| Card: Conversation Timeline                           |
|  ┌─ [Avatar] username (用户)       2026-05-03 10:00   |
|  │  工单初始描述内容...                                |
|  │                                                    |
|  ├─ [Avatar] admin_name (管理员)   2026-05-03 11:30   |
|  │  管理员回复内容...                                  |
|  │                                                    |
|  ├─ [Avatar] username (用户)       2026-05-03 12:00   |
|  │  用户回复内容...                                    |
|  │                                                    |
|  └─ (Timeline end)                                    |
+-------------------------------------------------------+
|                                                       |
| Card: Reply Input (hidden when status=closed)         |
|  [TextArea: 请输入回复内容...]                         |
|  [发送回复]                                            |
+-------------------------------------------------------+
```

### 2.5 Create Ticket Modal

```
+--------------------------------------------+
| Modal: 创建工单                             |
|                                            |
|  标题 *                                    |
|  [Input: 请输入工单标题]                    |
|                                            |
|  优先级                                    |
|  [Select: 低 / 普通(default) / 高]         |
|                                            |
|  描述 *                                    |
|  [TextArea: 请详细描述您的问题...]           |
|                                            |
|  [取消]                     [提交工单]      |
+--------------------------------------------+
```

---

## 3. Component Tree & Decomposition

### 3.1 Full Component Tree

```
pages/
  Ticket/
    index.jsx                         # TicketListPage wrapper (follows Token/Log pattern)

components/
  table/
    tickets/
      index.jsx                       # TicketsPage (orchestrator, like TokensPage)
      TicketsTable.jsx                # Table body rendering
      TicketsActions.jsx              # "Create Ticket" button
      TicketsFilters.jsx              # Status filter + keyword search
      TicketsDescription.jsx          # Description text above table
      modals/
        CreateTicketModal.jsx         # Create ticket modal form

  ticket-detail/
    index.jsx                         # TicketDetailPage (orchestrator)
    TicketHeader.jsx                  # Title, status, priority, metadata, close button
    TicketTimeline.jsx                # Timeline of replies
    TicketTimelineItem.jsx            # Single reply item in timeline
    TicketReplyBox.jsx                # TextArea + submit button

hooks/
  tickets/
    useTicketsData.jsx                # List page data (fetch, paginate, filter, search)
    useTicketDetail.jsx               # Detail page data (fetch ticket + replies)
```

### 3.2 Component Details

#### Component: `TicketsPage` (`components/table/tickets/index.jsx`)

**Responsibility**: Orchestrates the ticket list view. Mirrors the pattern in `TokensPage` and `LogsPage` -- uses `CardPro` with `type="type1"`, wires up description / actions / filters / pagination / table.

**Key dependencies**: `useTicketsData` hook, `CardPro`, `createCardProPagination`

**Skeleton structure**:

```jsx
function TicketsPage() {
  const ticketsData = useTicketsData();
  const isMobile = useIsMobile();

  return (
    <>
      <CreateTicketModal
        visible={ticketsData.showCreate}
        onClose={ticketsData.closeCreate}
        onSuccess={ticketsData.refresh}
      />

      <CardPro
        type="type1"
        descriptionArea={<TicketsDescription t={ticketsData.t} />}
        actionsArea={
          <div className="flex flex-col md:flex-row justify-between items-center gap-2 w-full">
            <TicketsActions
              onCreateClick={ticketsData.openCreate}
              t={ticketsData.t}
            />
            <TicketsFilters
              statusFilter={ticketsData.statusFilter}
              onStatusChange={ticketsData.setStatusFilter}
              searchKeyword={ticketsData.searchKeyword}
              onSearch={ticketsData.handleSearch}
              loading={ticketsData.loading}
              t={ticketsData.t}
            />
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: ticketsData.activePage,
          pageSize: ticketsData.pageSize,
          total: ticketsData.ticketCount,
          onPageChange: ticketsData.handlePageChange,
          onPageSizeChange: ticketsData.handlePageSizeChange,
          isMobile,
          t: ticketsData.t,
        })}
        t={ticketsData.t}
      >
        <TicketsTable {...ticketsData} />
      </CardPro>
    </>
  );
}
```

---

#### Component: `TicketsTable` (`components/table/tickets/TicketsTable.jsx`)

**Responsibility**: Render the Semi UI `<Table>` with ticket rows.

**Columns**:

| Column Key | Header (i18n) | Render | Width |
|------------|---------------|--------|-------|
| `title` | `t('标题')` | Clickable link to `/console/ticket/:id` | flex |
| `status` | `t('状态')` | `<Tag>` with color mapping | 100px |
| `priority` | `t('优先级')` | `<Tag>` with color mapping | 90px |
| `creator` | `t('创建者')` | `username` text (admin view only) | 120px |
| `updated_at` | `t('更新时间')` | Formatted date string | 160px |
| `actions` | `t('操作')` | "View" button | 80px |

**Status Tag color mapping**:

```javascript
const STATUS_TAG_MAP = {
  open:     { color: 'blue',   label: t('待处理') },
  replied:  { color: 'orange', label: t('已回复') },
  closed:   { color: 'grey',   label: t('已关闭') },
};
```

**Priority Tag color mapping**:

```javascript
const PRIORITY_TAG_MAP = {
  low:    { color: 'green',  label: t('低') },
  normal: { color: 'blue',   label: t('普通') },
  high:   { color: 'red',    label: t('高') },
};
```

**Column visibility logic**:

- `creator` column: only visible when `isAdmin()` returns `true`
- All other columns: always visible

---

#### Component: `TicketsActions` (`components/table/tickets/TicketsActions.jsx`)

**Responsibility**: "Create Ticket" button. Simple wrapper.

```jsx
function TicketsActions({ onCreateClick, t }) {
  return (
    <div className="flex gap-2">
      <Button theme="solid" type="primary" onClick={onCreateClick}>
        {t('创建工单')}
      </Button>
    </div>
  );
}
```

---

#### Component: `TicketsFilters` (`components/table/tickets/TicketsFilters.jsx`)

**Responsibility**: Status dropdown filter + keyword search input.

```jsx
function TicketsFilters({ statusFilter, onStatusChange, searchKeyword, onSearch, loading, t }) {
  return (
    <div className="flex gap-2 w-full md:w-auto">
      <Select
        value={statusFilter}
        onChange={onStatusChange}
        optionList={[
          { label: t('全部'), value: '' },
          { label: t('待处理'), value: 'open' },
          { label: t('已回复'), value: 'replied' },
          { label: t('已关闭'), value: 'closed' },
        ]}
        style={{ width: 120 }}
      />
      <Input
        placeholder={t('搜索标题')}
        value={searchKeyword}
        onChange={onSearch}
        suffix={<IconSearch />}
        showClear
        style={{ width: 200 }}
      />
    </div>
  );
}
```

---

#### Component: `CreateTicketModal` (`components/table/tickets/modals/CreateTicketModal.jsx`)

**Responsibility**: Modal form for creating a new ticket.

**Form fields**:

| Field | Component | Validation | Default |
|-------|-----------|------------|---------|
| `title` | `<Input>` | Required, max 200 chars | `""` |
| `priority` | `<Select>` | Required | `"normal"` |
| `content` | `<TextArea>` | Required, max 5000 chars | `""` |

**State**:

- `submitting: boolean` -- disables form + shows spinner on submit button

**Behavior**:

1. On open: reset form to defaults
2. On submit: validate -> POST `/api/ticket` -> on success: close modal + `showSuccess(t('工单创建成功'))` + refresh list
3. On cancel: close modal without side effects

---

#### Component: `TicketDetailPage` (`components/ticket-detail/index.jsx`)

**Responsibility**: Orchestrate the detail view. Fetches ticket data and replies via `useTicketDetail` hook. Composes TicketHeader + TicketTimeline + TicketReplyBox.

```jsx
function TicketDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const detail = useTicketDetail(id);

  if (detail.loading) return <Loading />;
  if (detail.error) return <ErrorDisplay message={detail.error} />;

  return (
    <div className="mt-[60px] px-2 flex flex-col gap-4">
      <Button
        icon={<IconArrowLeft />}
        type="tertiary"
        onClick={() => navigate('/console/ticket')}
      >
        {detail.t('返回列表')}
      </Button>

      <TicketHeader
        ticket={detail.ticket}
        onClose={detail.handleClose}
        canClose={detail.canClose}
        closing={detail.closing}
        t={detail.t}
      />

      <TicketTimeline
        replies={detail.replies}
        t={detail.t}
      />

      {detail.ticket.status !== 'closed' && (
        <TicketReplyBox
          onSubmit={detail.handleReply}
          submitting={detail.submitting}
          t={detail.t}
        />
      )}
    </div>
  );
}
```

---

#### Component: `TicketHeader` (`components/ticket-detail/TicketHeader.jsx`)

**Responsibility**: Display ticket metadata and admin actions.

**Visual structure**:

```jsx
<Card>
  <div className="flex justify-between items-start">
    <div>
      <Typography.Title heading={4}>{ticket.title}</Typography.Title>
      <Space>
        <Tag color={STATUS_TAG_MAP[ticket.status].color}>
          {STATUS_TAG_MAP[ticket.status].label}
        </Tag>
        <Tag color={PRIORITY_TAG_MAP[ticket.priority].color}>
          {PRIORITY_TAG_MAP[ticket.priority].label}
        </Tag>
      </Space>
      <div className="text-sm text-gray-500 mt-2">
        {t('创建者')}: {ticket.creator_name} | {t('创建时间')}: {formatTime(ticket.created_at)}
      </div>
    </div>
    {canClose && ticket.status !== 'closed' && (
      <Button type="danger" loading={closing} onClick={onClose}>
        {t('关闭工单')}
      </Button>
    )}
  </div>
</Card>
```

---

#### Component: `TicketTimeline` (`components/ticket-detail/TicketTimeline.jsx`)

**Responsibility**: Render the chronological list of replies using Semi UI `<Timeline>`.

**Distinction between user and admin replies**:

| Attribute | User Reply | Admin/Reseller Reply |
|-----------|-----------|---------------------|
| Alignment | Left | Left (same, but with badge) |
| Badge | `<Avatar>` with user initial | `<Avatar>` with admin initial + admin badge |
| Background | `var(--semi-color-fill-0)` | `var(--semi-color-primary-light-default)` |
| Label | `username` | `admin_name` + `[管理员]` / `[代理]` tag |

**Structure**:

```jsx
<Timeline>
  {/* First item is the original ticket description */}
  <Timeline.Item
    dot={<Avatar size="small">{ticket.creator_name[0]}</Avatar>}
    time={formatTime(ticket.created_at)}
  >
    <Card className="bg-[var(--semi-color-fill-0)]">
      <Typography.Text>{ticket.content}</Typography.Text>
    </Card>
  </Timeline.Item>

  {/* Subsequent replies */}
  {replies.map(reply => (
    <TicketTimelineItem key={reply.id} reply={reply} />
  ))}
</Timeline>
```

---

#### Component: `TicketTimelineItem` (`components/ticket-detail/TicketTimelineItem.jsx`)

**Responsibility**: Single reply node in the timeline.

**Props**:

```javascript
{
  reply: {
    id: number,
    user_id: number,
    username: string,
    role: 'user' | 'admin' | 'reseller',
    content: string,
    created_at: string,
  }
}
```

**Render logic**:

```jsx
const isStaff = reply.role === 'admin' || reply.role === 'reseller';
const bgClass = isStaff
  ? 'bg-[var(--semi-color-primary-light-default)]'
  : 'bg-[var(--semi-color-fill-0)]';

return (
  <Timeline.Item
    dot={<Avatar size="small">{reply.username[0]}</Avatar>}
    time={formatTime(reply.created_at)}
  >
    <div className="mb-1">
      <Typography.Text strong>{reply.username}</Typography.Text>
      {isStaff && (
        <Tag size="small" color="violet" className="ml-2">
          {reply.role === 'admin' ? t('管理员') : t('代理')}
        </Tag>
      )}
    </div>
    <Card className={bgClass} bodyStyle={{ padding: '12px 16px' }}>
      <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', margin: 0 }}>
        {reply.content}
      </Typography.Paragraph>
    </Card>
  </Timeline.Item>
);
```

---

#### Component: `TicketReplyBox` (`components/ticket-detail/TicketReplyBox.jsx`)

**Responsibility**: Input area for submitting a reply.

**Props**:

```javascript
{
  onSubmit: (content: string) => Promise<void>,
  submitting: boolean,
  t: (key: string) => string,
}
```

**State**:

- `content: string` -- controlled TextArea value

**Behavior**:

- Submit button disabled when `content.trim() === ''` or `submitting === true`
- On successful submit: clear TextArea

```jsx
<Card>
  <TextArea
    value={content}
    onChange={setContent}
    placeholder={t('请输入回复内容...')}
    autosize={{ minRows: 3, maxRows: 8 }}
    maxCount={5000}
  />
  <div className="flex justify-end mt-3">
    <Button
      theme="solid"
      type="primary"
      loading={submitting}
      disabled={!content.trim() || submitting}
      onClick={handleSubmit}
    >
      {t('发送回复')}
    </Button>
  </div>
</Card>
```

---

## 4. Interaction Flow

### 4.1 User Journey: Create and Track a Ticket

```
User enters /console/ticket
    |
    v
[Ticket List Page loads]
    |-- API: GET /api/ticket?page=1&page_size=10
    |-- Renders table with user's tickets (server-side scope filtering)
    |
    v
User clicks "创建工单"
    |
    v
[CreateTicketModal opens]
    |-- User fills: title, priority, content
    |-- Clicks "提交工单"
    |-- API: POST /api/ticket
    |       Body: { title, priority, content }
    |-- On success:
    |     Modal closes
    |     Toast: "工单创建成功"
    |     List refreshes
    |-- On error:
    |     Toast: error message
    |     Modal stays open
    |
    v
User clicks a ticket row (title link)
    |
    v
[Ticket Detail Page loads]
    |-- API: GET /api/ticket/:id
    |-- API: GET /api/ticket/:id/replies
    |-- Renders header + timeline + reply box
    |
    v
User types reply and clicks "发送回复"
    |-- API: POST /api/ticket/:id/reply
    |       Body: { content }
    |-- On success:
    |     TextArea clears
    |     Timeline appends new reply (re-fetch or optimistic append)
    |     Toast: "回复成功"
    |-- On error:
    |     Toast: error message
    |     Content preserved in TextArea
```

### 4.2 Admin Journey: Triage and Close

```
Admin enters /console/ticket
    |
    v
[Ticket List loads with all scoped tickets]
    |-- Admin sees "creator" column (extra column vs end_user view)
    |-- Admin can filter by status: open / replied / closed
    |
    v
Admin clicks a ticket row
    |
    v
[Ticket Detail Page]
    |-- Admin sees "关闭工单" button (shown only if canClose=true && status!='closed')
    |
    |-- Option A: Admin replies
    |     API: POST /api/ticket/:id/reply  { content }
    |     Ticket status auto-changes: open -> replied (server-side logic)
    |
    |-- Option B: Admin closes ticket
    |     Confirm dialog: "确定关闭此工单？关闭后用户将无法继续回复。"
    |     API: PUT /api/ticket/:id/close
    |     On success:
    |       Status tag updates to "已关闭" (grey)
    |       Reply box disappears
    |       Toast: "工单已关闭"
```

### 4.3 State Transition Table

| Current Status | Event | Next Status | UI Change |
|---------------|-------|-------------|-----------|
| `open` | Admin/Reseller replies | `replied` | Status tag: blue -> orange |
| `open` | Admin closes | `closed` | Tag -> grey; reply box hidden |
| `replied` | User replies | `open` | Status tag: orange -> blue |
| `replied` | Admin closes | `closed` | Tag -> grey; reply box hidden |
| `closed` | (no action available) | -- | Reply box not rendered |

Note: Status transitions are handled **server-side**. The frontend simply re-fetches and reflects the current state.

### 4.4 Permission-based UI Rendering

| UI Element | end_user | reseller_l2 | reseller_l1 | tenant_admin | platform_admin |
|-----------|----------|-------------|-------------|-------------|---------------|
| "创建工单" button | Visible | Visible | Visible | Visible | Visible |
| "Creator" table column | Hidden | Visible | Visible | Visible | Visible |
| "关闭工单" button | Hidden | Hidden | Hidden | Visible | Visible |
| Reply box (when open) | Visible | Visible | Visible | Visible | Visible |

Implementation: The API response for `GET /api/ticket/:id` should include a `permissions` object:

```json
{
  "can_reply": true,
  "can_close": true
}
```

The frontend renders UI elements conditionally based on this server-provided permission field, rather than computing permissions client-side. This avoids role logic duplication and ensures correctness when role definitions change.

---

## 5. State Management

### 5.1 Hook: `useTicketsData` (List page)

Located at `hooks/tickets/useTicketsData.jsx`. Follows the same pattern as `useTokensData`.

**Managed state**:

```javascript
{
  // Data
  tickets: [],              // Current page's ticket array
  ticketCount: 0,           // Total count for pagination
  loading: true,            // Initial load / page change loading

  // Pagination
  activePage: 1,
  pageSize: ITEMS_PER_PAGE, // Reuse existing constant (default 10)

  // Filters
  statusFilter: '',         // '' | 'open' | 'replied' | 'closed'
  searchKeyword: '',        // Title search string

  // Modal
  showCreate: false,

  // i18n
  t: useTranslation().t,
}
```

**Key functions**:

```javascript
{
  // Fetch
  loadTickets: async () => { ... },   // GET /api/ticket?page=N&page_size=M&status=X&keyword=Y
  refresh: () => loadTickets(),

  // Pagination
  handlePageChange: (page) => { setActivePage(page); },
  handlePageSizeChange: (size) => { setPageSize(size); setActivePage(1); },

  // Filter
  setStatusFilter: (status) => { setStatusFilter(status); setActivePage(1); },
  handleSearch: (keyword) => { setSearchKeyword(keyword); setActivePage(1); },

  // Modal
  openCreate: () => setShowCreate(true),
  closeCreate: () => setShowCreate(false),
}
```

**Data fetch trigger**: `useEffect` on `[activePage, pageSize, statusFilter, searchKeyword]`.

Search debounce: apply 300ms debounce on `handleSearch` to avoid excessive API calls during typing.

---

### 5.2 Hook: `useTicketDetail` (Detail page)

Located at `hooks/tickets/useTicketDetail.jsx`.

**Managed state**:

```javascript
{
  // Data
  ticket: null,             // Ticket metadata object
  replies: [],              // Array of reply objects
  loading: true,            // Initial load
  error: null,              // Error message string

  // Reply
  submitting: false,        // Reply submission in progress

  // Close
  closing: false,           // Close operation in progress
  canClose: false,          // Derived from ticket.permissions.can_close

  // i18n
  t: useTranslation().t,
}
```

**Key functions**:

```javascript
{
  // Fetch
  loadTicket: async () => { ... },     // GET /api/ticket/:id
  loadReplies: async () => { ... },    // GET /api/ticket/:id/replies

  // Actions
  handleReply: async (content) => {
    // POST /api/ticket/:id/reply { content }
    // On success: append to replies array, clear input
  },
  handleClose: async () => {
    // Confirmation modal first
    // PUT /api/ticket/:id/close
    // On success: update ticket.status, re-render
  },
}
```

---

## 6. API Contract (Frontend Expectations)

The frontend expects these API endpoints. Exact backend implementation is outside this document's scope, but the request/response shapes should match.

### 6.1 API Endpoint List

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/api/ticket` | List tickets (scoped) | Required |
| `POST` | `/api/ticket` | Create ticket | Required |
| `GET` | `/api/ticket/:id` | Get ticket detail + permissions | Required |
| `GET` | `/api/ticket/:id/replies` | Get ticket replies | Required |
| `POST` | `/api/ticket/:id/reply` | Add reply | Required |
| `PUT` | `/api/ticket/:id/close` | Close ticket | Required (admin/tenant_admin) |

### 6.2 Request/Response Shapes

#### `GET /api/ticket`

**Query params**:

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `page` | int | No (default 1) | Page number |
| `page_size` | int | No (default 10) | Items per page |
| `status` | string | No | Filter: `open`, `replied`, `closed` |
| `keyword` | string | No | Title search (fuzzy match) |

**Response**:

```json
{
  "success": true,
  "message": "",
  "data": {
    "tickets": [
      {
        "id": 1,
        "title": "API rate limit issue",
        "status": "open",
        "priority": "high",
        "creator_id": 42,
        "creator_name": "user01",
        "created_at": "2026-05-03T10:00:00Z",
        "updated_at": "2026-05-03T11:30:00Z"
      }
    ],
    "total": 25
  }
}
```

#### `POST /api/ticket`

**Body**:

```json
{
  "title": "string (required, max 200)",
  "priority": "low | normal | high",
  "content": "string (required, max 5000)"
}
```

**Response**:

```json
{
  "success": true,
  "message": "工单创建成功",
  "data": {
    "id": 2
  }
}
```

#### `GET /api/ticket/:id`

**Response**:

```json
{
  "success": true,
  "data": {
    "id": 1,
    "title": "API rate limit issue",
    "status": "open",
    "priority": "high",
    "content": "Original ticket description...",
    "creator_id": 42,
    "creator_name": "user01",
    "created_at": "2026-05-03T10:00:00Z",
    "updated_at": "2026-05-03T11:30:00Z",
    "permissions": {
      "can_reply": true,
      "can_close": true
    }
  }
}
```

#### `GET /api/ticket/:id/replies`

**Response**:

```json
{
  "success": true,
  "data": [
    {
      "id": 101,
      "user_id": 42,
      "username": "user01",
      "role": "user",
      "content": "Reply content...",
      "created_at": "2026-05-03T11:00:00Z"
    },
    {
      "id": 102,
      "user_id": 1,
      "username": "admin",
      "role": "admin",
      "content": "Admin reply...",
      "created_at": "2026-05-03T11:30:00Z"
    }
  ]
}
```

#### `POST /api/ticket/:id/reply`

**Body**:

```json
{
  "content": "string (required, max 5000)"
}
```

**Response**:

```json
{
  "success": true,
  "message": "回复成功",
  "data": {
    "id": 103,
    "user_id": 42,
    "username": "user01",
    "role": "user",
    "content": "Reply content...",
    "created_at": "2026-05-03T12:00:00Z"
  }
}
```

#### `PUT /api/ticket/:id/close`

**Response**:

```json
{
  "success": true,
  "message": "工单已关闭"
}
```

---

## 7. File Creation Checklist

### New files to create

```
web/classic/src/
  pages/
    Ticket/
      index.jsx                                    # Page wrapper

  components/
    table/
      tickets/
        index.jsx                                  # TicketsPage orchestrator
        TicketsTable.jsx                           # Table rendering
        TicketsActions.jsx                         # Action buttons
        TicketsFilters.jsx                         # Status filter + search
        TicketsDescription.jsx                     # Description area
        modals/
          CreateTicketModal.jsx                    # Create ticket modal

    ticket-detail/
      index.jsx                                    # TicketDetailPage orchestrator
      TicketHeader.jsx                             # Ticket metadata card
      TicketTimeline.jsx                           # Reply timeline
      TicketTimelineItem.jsx                       # Single timeline item
      TicketReplyBox.jsx                           # Reply input area

  hooks/
    tickets/
      useTicketsData.jsx                           # List page hook
      useTicketDetail.jsx                          # Detail page hook
```

### Existing files to modify

| File | Change |
|------|--------|
| `App.jsx` | Add 2 routes: `/console/ticket` and `/console/ticket/:id` |
| `components/layout/SiderBar.jsx` | Add `ticket` to `routerMap` and `workspaceItems` |
| `helpers/render.jsx` | Add `case 'ticket'` in `getLucideIcon` (use `TicketCheck` from lucide-react) |
| `hooks/common/useSidebar.js` | Add `ticket: true` to `DEFAULT_ADMIN_CONFIG.console` |

### i18n keys to add

```json
{
  "工单": "Tickets",
  "工单管理": "Ticket Management",
  "创建工单": "Create Ticket",
  "工单标题": "Ticket Title",
  "优先级": "Priority",
  "低": "Low",
  "普通": "Normal",
  "高": "High",
  "描述": "Description",
  "待处理": "Open",
  "已回复": "Replied",
  "已关闭": "Closed",
  "创建者": "Creator",
  "更新时间": "Updated",
  "创建时间": "Created",
  "查看": "View",
  "搜索标题": "Search by title",
  "返回列表": "Back to list",
  "关闭工单": "Close Ticket",
  "发送回复": "Send Reply",
  "请输入回复内容...": "Type your reply...",
  "请输入工单标题": "Enter ticket title",
  "请详细描述您的问题...": "Describe your issue in detail...",
  "提交工单": "Submit Ticket",
  "工单创建成功": "Ticket created",
  "回复成功": "Reply sent",
  "工单已关闭": "Ticket closed",
  "确定关闭此工单？关闭后用户将无法继续回复。": "Close this ticket? Users will not be able to reply after closing.",
  "管理员": "Admin",
  "代理": "Reseller"
}
```

---

## 8. Risk Points & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Permission bypass (user views others' tickets) | High | All scope filtering must be server-side. Frontend never trusts client role for data filtering. |
| Race condition on status (user replies while admin closes) | Medium | API returns 409 Conflict; frontend shows "工单已关闭，无法回复" and refreshes status. |
| Large ticket thread (100+ replies) | Low | Paginate replies API (add `page` param); initial load shows latest 50; "加载更多" button for older. |
| Real-time notification not implemented | Low | MVP uses polling or manual refresh. WebSocket can be added in v2. |

---

## 9. Rollback Plan

If the ticket feature needs to be disabled without code revert:

1. Set `ticket: false` in `DEFAULT_ADMIN_CONFIG.console` -- hides sidebar entry for all users
2. Backend: disable `/api/ticket*` routes via middleware flag
3. No database migration rollback needed (new tables only, no existing table changes)
