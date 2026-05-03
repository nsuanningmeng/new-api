/*
Copyright (C) 2025 QuantumNous
*/

import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Typography,
  Tag,
  Space,
  Button,
  Card,
  TextArea,
  Spin,
  Modal,
  Descriptions,
  List,
  Avatar,
  Divider,
  Select,
} from '@douyinfe/semi-ui';
import { IconUser, IconCustomerService, IconSetting } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showSuccess,
  isAdmin,
  isRoot,
  timestamp2string,
  getUserIdFromLocalStorage,
} from '../../helpers';

const { Title, Text, Paragraph } = Typography;

const TicketDetail = () => {
  const { t } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const [ticket, setTicket] = useState(null);
  const [replies, setReplies] = useState([]);
  const [loading, setLoading] = useState(true);
  const [replyContent, setReplyContent] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const currentUserId = getUserIdFromLocalStorage();

  const fetchDetail = useCallback(async () => {
    setLoading(true);
    try {
      const isAdminUser = isAdmin();
      const url = isAdminUser ? `/api/ticket_admin/${id}` : `/api/ticket/${id}`;
      const res = await API.get(url);
      if (res.data.success) {
        setTicket(res.data.data);
        setReplies(res.data.data.replies || []);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (e) {
      showError(e);
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    fetchDetail();
  }, [fetchDetail]);

  const handleReply = async () => {
    if (!replyContent.trim()) return;
    setSubmitting(true);
    try {
      const url = isAdmin() ? `/api/ticket_admin/${id}/reply` : `/api/ticket/${id}/reply`;
      const res = await API.post(url, { content: replyContent });
      if (res.data.success) {
        showSuccess(t('回复成功'));
        setReplyContent('');
        fetchDetail();
      } else {
        showError(res.data.message || t('回复失败'));
      }
    } catch (e) {
      showError(e);
    } finally {
      setSubmitting(false);
    }
  };

  const handleAction = (action) => {
    const isAdminUser = isAdmin();
    const basePath = isAdminUser ? `/api/ticket_admin` : `/api/ticket`;

    Modal.confirm({
      title: t(`确认${action === 'close' ? '关闭' : '升级'}工单?`),
      onOk: async () => {
        try {
          const res = await API.post(`${basePath}/${id}/${action}`);
          if (res.data.success) {
            showSuccess(t('操作成功'));
            fetchDetail();
          } else {
            showError(res.data.message || t('操作失败'));
          }
        } catch (e) {
          showError(e);
        }
      },
    });
  };

  const handleAssign = () => {
    let targetRole = 'platform_admin';
    Modal.confirm({
      title: t('转派工单'),
      content: (
        <div style={{ marginTop: 10 }}>
          <Text>{t('选择目标角色')}:</Text>
          <Select
            style={{ width: '100%', marginTop: 10 }}
            defaultValue={targetRole}
            onChange={(v) => (targetRole = v)}
          >
            <Select.Option value='platform_admin'>{t('platform_admin')}</Select.Option>
            <Select.Option value='tenant_admin'>{t('tenant_admin')}</Select.Option>
            <Select.Option value='reseller_l1'>{t('reseller_l1')}</Select.Option>
            <Select.Option value='reseller_l2'>{t('reseller_l2')}</Select.Option>
          </Select>
        </div>
      ),
      onOk: async () => {
        try {
          const res = await API.post(`/api/ticket_admin/${id}/assign`, { target_role: targetRole });
          if (res.data.success) {
            showSuccess(t('转派成功'));
            fetchDetail();
          } else {
            showError(res.data.message || t('转派失败'));
          }
        } catch (e) {
          showError(e);
        }
      },
    });
  };

  if (loading && !ticket) return <Spin size='large' style={{ display: 'block', margin: '100px auto' }} />;
  if (!ticket) return null;

  const isOwner = ticket.user_id === currentUserId;
  const canEscalate = isOwner || isAdmin();
  const canAssign = isAdmin() || isRoot();
  const canClose = ticket.status !== 'closed' && (isOwner || isAdmin());

  const metaData = [
    { key: t('提交者 ID'), value: ticket.user_id },
    { key: t('租户 ID'), value: ticket.tenant_id },
    { key: t('当前处理'), value: t(ticket.assignee_role) },
    { key: t('处理层级'), value: ticket.assignee_level },
    { key: t('升级次数'), value: ticket.escalate_count },
    { key: t('创建时间'), value: timestamp2string(ticket.created_at) },
  ];

  return (
    <div style={{ padding: '20px', marginTop: '60px' }}>
      <Card>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            marginBottom: '20px',
            flexWrap: 'wrap',
            gap: '10px',
          }}
        >
          <Space vertical align='start'>
            <Title heading={3}>
              #{ticket.id} {ticket.subject}
            </Title>
            <Space>
              <Tag color={ticket.status === 'open' ? 'blue' : ticket.status === 'replied' ? 'green' : 'grey'}>
                {t(ticket.status)}
              </Tag>
              <Tag color={ticket.priority === 'high' ? 'red' : ticket.priority === 'normal' ? 'green' : 'cyan'}>
                {t(ticket.priority)}
              </Tag>
            </Space>
          </Space>
          <Space>
            {canEscalate && (
              <Button theme='solid' type='warning' onClick={() => handleAction('escalate')}>
                {t('升级')}
              </Button>
            )}
            {canAssign && (
              <Button theme='solid' type='secondary' onClick={handleAssign}>
                {t('转派')}
              </Button>
            )}
            {canClose && (
              <Button theme='solid' type='danger' onClick={() => handleAction('close')}>
                {t('关闭')}
              </Button>
            )}
            <Button onClick={() => navigate(-1)}>{t('返回')}</Button>
          </Space>
        </div>

        <Descriptions data={metaData} row size='small' style={{ marginBottom: '20px' }} />

        <Divider>{t('对话记录')}</Divider>

        <List
          dataSource={replies}
          renderItem={(item) => (
            <List.Item
              style={{
                justifyContent: item.is_admin ? 'flex-start' : 'flex-end',
                backgroundColor: item.is_system ? 'var(--semi-color-fill-0)' : 'transparent',
                padding: '10px',
                borderRadius: '8px',
              }}
            >
              <Space
                align='start'
                style={{
                  flexDirection: item.is_admin ? 'row' : 'row-reverse',
                  width: '100%',
                }}
              >
                <Avatar
                  size='small'
                  color={item.is_system ? 'orange' : item.is_admin ? 'blue' : 'green'}
                >
                  {item.is_system ? <IconSetting /> : item.is_admin ? <IconCustomerService /> : <IconUser />}
                </Avatar>
                <div style={{ maxWidth: '80%' }}>
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: item.is_admin ? 'flex-start' : 'flex-end',
                      gap: '8px',
                    }}
                  >
                    <Text strong size='small'>
                      {item.is_system
                        ? t('系统')
                        : item.is_admin
                        ? `${t('客服')} (${t(item.actor_role)})`
                        : t('用户')}
                    </Text>
                    <Text size='small' type='tertiary'>
                      {timestamp2string(item.created_at)}
                    </Text>
                  </div>
                  <Card bodyStyle={{ padding: '8px 12px' }} style={{ marginTop: '4px' }}>
                    <Paragraph style={{ whiteSpace: 'pre-wrap' }}>{item.content}</Paragraph>
                  </Card>
                </div>
              </Space>
            </List.Item>
          )}
        />

        <div style={{ marginTop: '20px' }}>
          <Divider>{t('附件')}</Divider>
          <div style={{ padding: '10px', color: 'var(--semi-color-text-2)' }}>
            <Text type='secondary'>{t('附件功能 (Phase 2)')}</Text>
            {ticket.attachments && ticket.attachments.length > 0 && (
              <ul style={{ marginTop: 10 }}>
                {ticket.attachments.map((a, i) => (
                  <li key={i}>{a.filename || `file_${i}`}</li>
                ))}
              </ul>
            )}
          </div>
        </div>

        <div style={{ marginTop: '20px' }}>
          <TextArea
            rows={4}
            value={replyContent}
            onChange={setReplyContent}
            placeholder={ticket.status === 'closed' ? t('该工单已关闭，无法回复') : t('请输入回复内容...')}
            disabled={ticket.status === 'closed'}
            maxLength={20000}
          />
          <div style={{ marginTop: '10px', display: 'flex', justifyContent: 'flex-end' }}>
            <Button
              type='primary'
              theme='solid'
              onClick={handleReply}
              loading={submitting}
              disabled={ticket.status === 'closed' || !replyContent.trim()}
            >
              {t('发送回复')}
            </Button>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default TicketDetail;
