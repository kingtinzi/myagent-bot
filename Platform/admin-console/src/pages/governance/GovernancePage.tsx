import { useEffect, useMemo, useState } from 'react';

import { StatusBadge } from '../../components/display/StatusBadge';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { DataRetentionPolicyPanel } from '../../features/governance/DataRetentionPolicyPanel';
import { RiskRulePanel } from '../../features/governance/RiskRulePanel';
import { SystemNoticePanel } from '../../features/governance/SystemNoticePanel';
import { useCapabilities } from '../../hooks/useCapabilities';

type GovernanceTabKey = 'notices' | 'risk' | 'retention';

type GovernanceTabDefinition = {
  key: GovernanceTabKey;
  label: string;
  description: string;
  hint: string;
  readCapability: string;
  writeCapability: string;
  tone: 'info' | 'warning' | 'danger';
};

const governanceTabs: GovernanceTabDefinition[] = [
  {
    key: 'notices',
    label: '系统公告',
    description: '维护面向用户展示的横幅公告、充值说明和治理提醒。',
    hint: '适合运营和治理管理员日常发布提醒。',
    readCapability: 'notices.read',
    writeCapability: 'notices.write',
    tone: 'info',
  },
  {
    key: 'risk',
    label: '风控规则',
    description: '集中维护风控检查规则、人工审核策略和治理标识。',
    hint: '高风险配置，影响风控与治理执行口径。',
    readCapability: 'risk.read',
    writeCapability: 'risk.write',
    tone: 'danger',
  },
  {
    key: 'retention',
    label: '数据保留策略',
    description: '定义关键数据域的保留天数、清理模式和策略说明。',
    hint: '策略变化会影响账本、审计和证据链的保留周期。',
    readCapability: 'retention.read',
    writeCapability: 'retention.write',
    tone: 'warning',
  },
];

function renderPanel(key: GovernanceTabKey, canWrite: boolean) {
  switch (key) {
    case 'notices':
      return <SystemNoticePanel canWrite={canWrite} />;
    case 'risk':
      return <RiskRulePanel canWrite={canWrite} />;
    case 'retention':
      return <DataRetentionPolicyPanel canWrite={canWrite} />;
    default:
      return null;
  }
}

export function GovernancePage() {
  const capabilities = useCapabilities();
  const visibleTabs = useMemo(
    () =>
      governanceTabs.filter(
        tab => capabilities.hasCapability(tab.readCapability) || capabilities.hasCapability(tab.writeCapability),
      ),
    [capabilities],
  );

  const [activeTab, setActiveTab] = useState<GovernanceTabKey | null>(() => visibleTabs[0]?.key ?? null);

  useEffect(() => {
    if (visibleTabs.length === 0) {
      setActiveTab(null);
      return;
    }

    if (!activeTab || !visibleTabs.some(tab => tab.key === activeTab)) {
      setActiveTab(visibleTabs[0].key);
    }
  }, [activeTab, visibleTabs]);

  const currentTab = visibleTabs.find(tab => tab.key === activeTab) ?? visibleTabs[0] ?? null;
  const writableCount = visibleTabs.filter(tab => capabilities.hasCapability(tab.writeCapability)).length;
  const currentCanWrite = currentTab ? capabilities.hasCapability(currentTab.writeCapability) : false;

  if (!currentTab) {
    return <InlineStatus tone="warning">当前管理员没有查看治理工作台细分能力，请联系超级管理员授权。</InlineStatus>;
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="治理工作台"
        title="公告、风控与保留策略"
        description="围绕线上公告、风险规则和数据保留策略建立统一治理工作台，让配置边界更清晰、审计反馈更直接。"
        meta={
          <>
            <StatusBadge tone="info">{visibleTabs.length} 个治理域</StatusBadge>
            <StatusBadge tone={writableCount > 0 ? 'success' : 'warning'}>
              {writableCount > 0 ? `可写 ${writableCount} 项` : '仅可巡检'}
            </StatusBadge>
          </>
        }
      />

      <section className="panel catalog-workspace governance-workspace">
        <div className="panel__header">
          <div>
            <h2>按治理对象分区编辑</h2>
            <p>治理模块优先强调可读性和风险提示；高影响动作仍保留统一确认流与 revision 并发保护。</p>
          </div>
          <StatusBadge tone={currentCanWrite ? 'success' : 'warning'}>
            {currentCanWrite ? '当前分区可编辑' : '当前分区只读'}
          </StatusBadge>
        </div>

        <div aria-label="治理配置分区" className="segmented-control" role="tablist">
          {visibleTabs.map(tab => (
            <button
              aria-selected={tab.key === currentTab.key}
              className={['segmented-control__button', tab.key === currentTab.key ? 'is-active' : ''].filter(Boolean).join(' ')}
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              role="tab"
              type="button"
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="catalog-summary-grid">
          <article className="catalog-summary-card">
            <span>当前分区</span>
            <strong>{currentTab.label}</strong>
            <p>{currentTab.description}</p>
          </article>
          <article className="catalog-summary-card">
            <span>操作提示</span>
            <strong>{currentCanWrite ? '保存前请复核影响面' : '只读巡检模式'}</strong>
            <p>{currentTab.hint}</p>
          </article>
          <article className="catalog-summary-card">
            <span>风险级别</span>
            <strong>{currentTab.tone === 'danger' ? '高风险' : currentTab.tone === 'warning' ? '需复核' : '常规治理'}</strong>
            <StatusBadge tone={currentTab.tone}>权限隔离</StatusBadge>
          </article>
        </div>
      </section>

      {renderPanel(currentTab.key, currentCanWrite)}
    </section>
  );
}
