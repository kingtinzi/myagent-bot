import { useEffect, useMemo, useState } from 'react';

import { StatusBadge } from '../../components/display/StatusBadge';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { useCapabilities } from '../../hooks/useCapabilities';
import { AgreementEditorPanel } from '../../features/catalog/AgreementEditorPanel';
import { ModelListPanel } from '../../features/catalog/ModelListPanel';
import { PricingEditorPanel } from '../../features/catalog/PricingEditorPanel';
import { RouteEditorPanel } from '../../features/catalog/RouteEditorPanel';
import { RuntimeConfigPanel } from '../../features/catalog/RuntimeConfigPanel';

type CatalogTabKey = 'models' | 'routes' | 'pricing' | 'agreements' | 'runtime';

type CatalogTabDefinition = {
  key: CatalogTabKey;
  label: string;
  description: string;
  hint: string;
  readCapability: string;
  writeCapability: string;
  tone: 'info' | 'success' | 'warning' | 'danger';
};

const catalogTabs: CatalogTabDefinition[] = [
  {
    key: 'models',
    label: '官方模型',
    description: '维护前台可见的官方模型目录、模型描述与默认计费版本。',
    hint: '适合产品运营、模型运营和日常目录维护。',
    readCapability: 'models.read',
    writeCapability: 'models.write',
    tone: 'info',
  },
  {
    key: 'routes',
    label: '官方路由',
    description: '维护协议类型、上游模型 ID、API Base、密钥占位与额外 model_config 字段。',
    hint: '高风险配置。路由变更会直接影响真实模型调用链路。',
    readCapability: 'routes.read',
    writeCapability: 'routes.write',
    tone: 'danger',
  },
  {
    key: 'pricing',
    label: '定价规则',
    description: '维护 token 定价、兜底价格与生效时间，直接影响钱包扣费结果。',
    hint: '建议与财务/运营联合复核后再发布。',
    readCapability: 'pricing.read',
    writeCapability: 'pricing.write',
    tone: 'warning',
  },
  {
    key: 'agreements',
    label: '协议版本',
    description: '维护用户协议、隐私政策与充值前知情材料的内容版本。',
    hint: '发布后会直接影响注册和充值前的用户展示内容。',
    readCapability: 'agreements.read',
    writeCapability: 'agreements.write',
    tone: 'warning',
  },
  {
    key: 'runtime',
    label: '运行时快照',
    description: '在一个原始 JSON 视图中检查或覆盖全量目录状态。',
    hint: '仅用于迁移、审计、紧急恢复等高级场景。',
    readCapability: 'runtime.read',
    writeCapability: 'runtime.write',
    tone: 'danger',
  },
];

function renderPanel(key: CatalogTabKey, canWrite: boolean) {
  switch (key) {
    case 'models':
      return <ModelListPanel canWrite={canWrite} />;
    case 'routes':
      return <RouteEditorPanel canWrite={canWrite} />;
    case 'pricing':
      return <PricingEditorPanel canWrite={canWrite} />;
    case 'agreements':
      return <AgreementEditorPanel canWrite={canWrite} />;
    case 'runtime':
      return <RuntimeConfigPanel canWrite={canWrite} />;
    default:
      return null;
  }
}

export function CatalogPage() {
  const capabilities = useCapabilities();
  const visibleTabs = useMemo(
    () =>
      catalogTabs.filter(
        tab => capabilities.hasCapability(tab.readCapability) || capabilities.hasCapability(tab.writeCapability),
      ),
    [capabilities],
  );

  const [activeTab, setActiveTab] = useState<CatalogTabKey | null>(() => visibleTabs[0]?.key ?? null);

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
    return <InlineStatus tone="warning">当前管理员没有查看目录配置台的细分权限，请联系超级管理员授权。</InlineStatus>;
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="目录工作台"
        title="官方模型与协议目录"
        description="将官方模型、路由、价格、协议与运行时快照收敛到同一套可审计工作台，所有保存都带 revision 并发保护。"
        meta={
          <>
            <StatusBadge tone="info">{visibleTabs.length} 个配置域</StatusBadge>
            <StatusBadge tone={writableCount > 0 ? 'success' : 'warning'}>
              {writableCount > 0 ? `可写 ${writableCount} 项` : '仅可巡检'}
            </StatusBadge>
          </>
        }
      />

      <section className="panel catalog-workspace">
        <div className="panel__header">
          <div>
            <h2>按配置域分区编辑</h2>
            <p>优先使用结构化面板完成变更；仅在需要批量迁移或审计时使用运行时快照高级模式。</p>
          </div>
          <StatusBadge tone={currentCanWrite ? 'success' : 'warning'}>
            {currentCanWrite ? '当前分区可编辑' : '当前分区只读'}
          </StatusBadge>
        </div>

        <div aria-label="目录配置分区" className="segmented-control" role="tablist">
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
            <strong>{currentCanWrite ? '保存前请复核' : '只读巡检模式'}</strong>
            <p>{currentTab.hint}</p>
          </article>
          <article className="catalog-summary-card">
            <span>风险级别</span>
            <strong>{currentTab.tone === 'danger' ? '高风险' : currentTab.tone === 'warning' ? '需复核' : '常规维护'}</strong>
            <StatusBadge tone={currentTab.tone}>按权限开放</StatusBadge>
          </article>
        </div>
      </section>

      {renderPanel(currentTab.key, currentCanWrite)}
    </section>
  );
}
