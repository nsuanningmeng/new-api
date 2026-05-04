/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsAvailability(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    'availability.thresholds.green': 99,
    'availability.thresholds.red': 95,
    'availability.count_status': '500-599,429',
    'availability.exclude_keywords': '',
    'availability.flush_seconds': 10,
    'availability.window_seconds': 3600,
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((inputs) => ({ ...inputs, [fieldName]: value }));
    };
  }

  function onSubmit() {
    // Validation
    if (
      inputs['availability.thresholds.red'] < 0 ||
      inputs['availability.thresholds.red'] > inputs['availability.thresholds.green'] ||
      inputs['availability.thresholds.green'] > 100
    ) {
      return showError(t('阈值无效：0 ≤ 红色 ≤ 绿色 ≤ 100'));
    }
    if (inputs['availability.flush_seconds'] < 5 || inputs['availability.flush_seconds'] > 3600) {
      return showError(t('刷新间隔必须在 5 到 3600 秒之间'));
    }
    if (inputs['availability.window_seconds'] < 60 || inputs['availability.window_seconds'] > 2592000) {
      return showError(t('统计窗口必须在 60 到 2592000 秒之间'));
    }

    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    // Special handling for thresholds
    const hasThresholdUpdate = updateArray.some((item) =>
      ['availability.thresholds.green', 'availability.thresholds.red'].includes(item.key)
    );

    const requestQueue = [];

    if (hasThresholdUpdate) {
      const thresholds = JSON.stringify({
        green: Number(inputs['availability.thresholds.green']),
        red: Number(inputs['availability.thresholds.red']),
      });
      requestQueue.push(API.put('/api/option/', { key: 'availability.thresholds', value: thresholds }));
    }

    updateArray.forEach((item) => {
      if (!['availability.thresholds.green', 'availability.thresholds.red'].includes(item.key)) {
        requestQueue.push(API.put('/api/option/', { key: item.key, value: String(inputs[item.key]) }));
      }
    });

    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined)) return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = {};
    // Load standard keys
    const keys = [
      'availability.count_status',
      'availability.exclude_keywords',
      'availability.flush_seconds',
      'availability.window_seconds',
    ];
    keys.forEach((key) => {
      if (props.options[key] !== undefined) {
        currentInputs[key] = props.options[key];
      }
    });

    // Load and parse thresholds
    if (props.options['availability.thresholds']) {
      try {
        const thresholds = JSON.parse(props.options['availability.thresholds']);
        currentInputs['availability.thresholds.green'] = thresholds.green;
        currentInputs['availability.thresholds.red'] = thresholds.red;
      } catch (e) {
        console.error('Failed to parse availability.thresholds', e);
      }
    }

    setInputs((prev) => ({ ...prev, ...currentInputs }));
    setInputsRow((prev) => ({ ...prev, ...currentInputs }));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('可用性设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'availability.thresholds.green'}
                  label={t('可用性阈值（绿色，%）')}
                  placeholder={'99'}
                  onChange={handleFieldChange('availability.thresholds.green')}
                  min={0}
                  max={100}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'availability.thresholds.red'}
                  label={t('可用性阈值（红色，%）')}
                  placeholder={'95'}
                  onChange={handleFieldChange('availability.thresholds.red')}
                  min={0}
                  max={100}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  field={'availability.count_status'}
                  label={t('计入失败的状态码（如 500-599,429）')}
                  placeholder={'500-599,429'}
                  onChange={handleFieldChange('availability.count_status')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Input
                  field={'availability.exclude_keywords'}
                  label={t('排除分组关键词（逗号分隔，支持中文）')}
                  placeholder={t('排除分组关键词（逗号分隔，支持中文）')}
                  onChange={handleFieldChange('availability.exclude_keywords')}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'availability.flush_seconds'}
                  label={t('刷新间隔（秒）')}
                  placeholder={'10'}
                  onChange={handleFieldChange('availability.flush_seconds')}
                  min={5}
                  max={3600}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  field={'availability.window_seconds'}
                  label={t('统计窗口（秒）')}
                  placeholder={'3600'}
                  onChange={handleFieldChange('availability.window_seconds')}
                  min={60}
                  max={2592000}
                />
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存可用性设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
