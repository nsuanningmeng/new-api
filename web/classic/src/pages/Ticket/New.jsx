/*
Copyright (C) 2025 QuantumNous
*/

import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Button, Card, Typography, Space, Upload } from '@douyinfe/semi-ui';
import { IconFile } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { compressImage, validateImageFile, uploadAttachment } from '../../helpers/ticket-upload';

const { Title, Text } = Typography;

const TicketNew = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  // 新建工单时还没有 ticket id，先把文件存在内存里，提交后再上传
  const [selectedFiles, setSelectedFiles] = useState([]);

  const handleSubmit = async (values) => {
    setLoading(true);
    let ticketId = null;
    try {
      const res = await API.post('/api/ticket/', values);
      if (res.data.success) {
        ticketId = res.data.data.id;
        showSuccess(t('工单创建成功'));

        if (selectedFiles.length > 0) {
          for (const file of selectedFiles) {
            try {
              const compressed = await compressImage(file);
              await uploadAttachment(ticketId, compressed);
            } catch (err) {
              showError(`${t('上传失败')}: ${file.name} — ${err.message || ''}`);
            }
          }
        }
        navigate(`/ticket/${ticketId}`);
      } else {
        showError(res.data.message || t('提交失败'));
      }
    } catch (e) {
      showError(e);
      // 工单已创建但附件失败的场景，也要把用户带去详情页继续操作
      if (ticketId) navigate(`/ticket/${ticketId}`);
    } finally {
      setLoading(false);
    }
  };

  const handleFileChange = ({ fileList }) => {
    const files = (fileList || [])
      .map((f) => f.fileInstance)
      .filter(Boolean)
      .filter((f) => {
        const v = validateImageFile(f);
        if (!v.ok) showError(`${f.name}: ${t(v.error)}`);
        return v.ok;
      });
    setSelectedFiles(files.slice(0, 5));
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
          <Form.Select field='category' label={t('分类')} style={{ width: '100%' }} initValue='general'>
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

          <div style={{ marginTop: '20px' }}>
            <Text strong>{t('附件')} (Max 5)</Text>
            <Upload
              action=''
              limit={5}
              accept='image/png,image/jpeg,image/gif,image/webp'
              onChange={handleFileChange}
              beforeUpload={() => false}
              style={{ marginTop: 10 }}
            >
              <Button icon={<IconFile />} theme='outline'>
                {t('选择附件')}
              </Button>
            </Upload>
          </div>

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
