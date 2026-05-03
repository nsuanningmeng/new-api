/*
Copyright (C) 2025 QuantumNous
*/

import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Button, Card, Typography, Space } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const { Title } = Typography;

const TicketNew = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values) => {
    setLoading(true);
    try {
      const res = await API.post('/api/ticket/', values);
      if (res.data.success) {
        showSuccess(t('工单创建成功'));
        navigate(`/ticket/${res.data.data.id}`);
      } else {
        showError(res.data.message || t('提交失败'));
      }
    } catch (e) {
      showError(e);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ padding: '20px', marginTop: '60px', maxWidth: '800px', margin: '60px auto' }}>
      <Card>
        <Title heading={3} style={{ marginBottom: '20px' }}>
          {t('提交新工单')}
        </Title>
        <Form onSubmit={handleSubmit} labelPosition='top'>
          <Form.Input
            field='subject'
            label={t('主题')}
            placeholder={t('简述您的问题')}
            rules={[{ required: true, message: t('请输入主题') }]}
            maxLength={255}
          />
          <Form.Select field='category' label={t('分类')} style={{ width: '100%' }} initValue=''>
            <Form.Select.Option value='general'>{t('通用')}</Form.Select.Option>
            <Form.Select.Option value='billing'>{t('计费')}</Form.Select.Option>
            <Form.Select.Option value='technical'>{t('技术')}</Form.Select.Option>
          </Form.Select>
          <Form.Select field='priority' label={t('优先级')} style={{ width: '100%' }} initValue='normal'>
            <Form.Select.Option value='low'>{t('低')}</Form.Select.Option>
            <Form.Select.Option value='normal'>{t('普通')}</Form.Select.Option>
            <Form.Select.Option value='high'>{t('高')}</Form.Select.Option>
          </Form.Select>
          <Form.TextArea
            field='content'
            label={t('描述')}
            placeholder={t('请详细描述您遇到的问题...')}
            rules={[{ required: true, message: t('请输入描述') }]}
            rows={8}
            maxLength={20000}
          />
          <Form.Checkbox field='direct_to_platform' noLabel>
            {t('需要平台介入')}
          </Form.Checkbox>

          <div style={{ marginTop: '20px', display: 'flex', justifyContent: 'flex-end' }}>
            <Space>
              <Button type='secondary' onClick={() => navigate(-1)}>
                {t('取消')}
              </Button>
              <Button type='primary' htmlType='submit' loading={loading}>
                {t('提交')}
              </Button>
            </Space>
          </div>
        </Form>
      </Card>
    </div>
  );
};

export default TicketNew;
