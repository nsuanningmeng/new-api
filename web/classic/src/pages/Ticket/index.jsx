/*
Copyright (C) 2025 QuantumNous
*/

import React, { useState, useEffect, useCallback } from 'react';
import { Table, Tag, Button, Tabs, Select, Pagination, Typography, Card } from '@douyinfe/semi-ui';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API, showError, isAdmin, timestamp2string } from '../../helpers';

const { Title } = Typography;

const TicketIndex = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState('mine');
  const [tickets, setTickets] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [statusFilter, setStatusFilter] = useState('all');

  const fetchTickets = useCallback(async () => {
    setLoading(true);
    try {
      let url = '/api/ticket/';
      if (activeTab === 'inbox') url = '/api/ticket_admin/inbox';
      if (activeTab === 'downstream') url = '/api/ticket_admin/downstream';

      const params = { page, page_size: pageSize };
      if (statusFilter !== 'all') params.status = statusFilter;

      const res = await API.get(url, { params });
      const { data } = res.data;
      if (data) {
        setTickets(data.items || []);
        setTotal(data.total || 0);
      }
    } catch (e) {
      showError(e);
    } finally {
      setLoading(false);
    }
  }, [activeTab, page, pageSize, statusFilter]);

  useEffect(() => {
    fetchTickets();
  }, [fetchTickets]);

  // 切换 Tab 时重置到第一页
  useEffect(() => {
    setPage(1);
  }, [activeTab, statusFilter]);

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: t('主题'), dataIndex: 'subject' },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (text) => {
        const colorMap = { open: 'blue', replied: 'green', closed: 'grey' };
        return <Tag color={colorMap[text] || 'blue'}>{t(text)}</Tag>;
      },
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      render: (text) => {
        const colorMap = { low: 'cyan', normal: 'green', high: 'red' };
        return <Tag color={colorMap[text] || 'green'}>{t(text)}</Tag>;
      },
    },
    {
      title: t('当前指派'),
      dataIndex: 'assignee_role',
      render: (text) => t(text || 'unassigned'),
    },
    {
      title: t('最后更新'),
      dataIndex: 'updated_at',
      render: (text) => timestamp2string(text),
    },
    {
      title: t('附件'),
      dataIndex: 'attachment_count',
      width: 80,
    },
    {
      title: t('操作'),
      render: (_, record) => (
        <Button theme='light' type='primary' onClick={() => navigate(`/ticket/${record.id}`)}>
          {t('查看')}
        </Button>
      ),
    },
  ];

  return (
    <div style={{ padding: '20px', marginTop: '60px' }}>
      <Card>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            marginBottom: '20px',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: '10px',
          }}
        >
          <Title heading={3}>{t('工单系统')}</Title>
          <Button type='primary' onClick={() => navigate('/ticket/new')}>
            {t('新建工单')}
          </Button>
        </div>

        <Tabs activeKey={activeTab} onChange={setActiveTab}>
          <Tabs.TabPane tab={t('我提交的')} itemKey='mine' />
          {isAdmin() && (
            <>
              <Tabs.TabPane tab={t('分给我的')} itemKey='inbox' />
              <Tabs.TabPane tab={t('下游监控')} itemKey='downstream' />
            </>
          )}
        </Tabs>

        <div style={{ marginTop: '20px', display: 'flex', gap: '10px', marginBottom: '10px' }}>
          <Select
            value={statusFilter}
            onChange={setStatusFilter}
            style={{ width: 150 }}
            placeholder={t('筛选状态')}
          >
            <Select.Option value='all'>{t('全部状态')}</Select.Option>
            <Select.Option value='open'>{t('待处理')}</Select.Option>
            <Select.Option value='replied'>{t('已回复')}</Select.Option>
            <Select.Option value='closed'>{t('已关闭')}</Select.Option>
          </Select>
        </div>

        <Table
          columns={columns}
          dataSource={tickets}
          loading={loading}
          pagination={false}
          rowKey='id'
        />

        <div style={{ marginTop: '20px', display: 'flex', justifyContent: 'flex-end' }}>
          <Pagination
            total={total}
            currentPage={page}
            pageSize={pageSize}
            onPageChange={setPage}
          />
        </div>
      </Card>
    </div>
  );
};

export default TicketIndex;
