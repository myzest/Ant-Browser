import { Link } from 'react-router-dom'
import { AlertCircle, ExternalLink, ShieldAlert, ShieldCheck, AlertTriangle, XCircle } from 'lucide-react'
import { Alert, Badge, Button, FormItem, Input, Modal } from '../../../../shared/components'
import { KeywordsModal } from '../../components/KeywordsModal'
import type { BrowserProfile } from '../../types'
import type { BrowserStartPreflightResult, BrowserStartWarningSeverity } from '../../utils/startPreflight'

interface BrowserListDialogsProps {
  proxyErrorModal: boolean
  pendingStartId: string | null
  proxyErrorMsg: string
  onCloseProxyError: () => void
  onStartDirect: () => void
  startingDirect: boolean
  kwModal: { open: boolean; profile: BrowserProfile | null }
  onCloseKeywords: () => void
  onKeywordsSaved: (keywords: string[]) => void
  expandModalOpen: boolean
  onCloseExpand: () => void
  profilesCount: number
  maxProfileLimit: number
  cdKey: string
  onCdKeyChange: (value: string) => void
  onRedeem: () => void
  redeeming: boolean
  onOpenGithubStarGift: () => void
  copyModal: { open: boolean; profile: BrowserProfile | null }
  copyName: string
  onCopyNameChange: (value: string) => void
  onCloseCopy: () => void
  onConfirmCopy: () => void
  copying: boolean
  opError: string
  onCloseOpError: () => void
  startPreflightModal: {
    open: boolean
    profile: BrowserProfile | null
    mode: 'normal' | 'direct'
    loading: boolean
    result: BrowserStartPreflightResult | null
    acknowledged: boolean
  }
  onCloseStartPreflight: () => void
  onAcknowledgeStartPreflight: (value: boolean) => void
  onConfirmStartPreflight: () => void | Promise<void>
}

const SEVERITY_STYLES: Record<BrowserStartWarningSeverity, { icon: typeof AlertCircle; badge: 'default' | 'info' | 'warning' | 'error'; label: string }> = {
  info: { icon: AlertCircle, badge: 'info', label: '提示' },
  warning: { icon: AlertTriangle, badge: 'warning', label: '警告' },
  error: { icon: ShieldAlert, badge: 'error', label: '严重' },
}

export function BrowserListDialogs({
  proxyErrorModal,
  pendingStartId,
  proxyErrorMsg,
  onCloseProxyError,
  onStartDirect,
  startingDirect,
  kwModal,
  onCloseKeywords,
  onKeywordsSaved,
  expandModalOpen,
  onCloseExpand,
  profilesCount,
  maxProfileLimit,
  cdKey,
  onCdKeyChange,
  onRedeem,
  redeeming,
  onOpenGithubStarGift,
  copyModal,
  copyName,
  onCopyNameChange,
  onCloseCopy,
  onConfirmCopy,
  copying,
  opError,
  onCloseOpError,
  startPreflightModal,
  onCloseStartPreflight,
  onAcknowledgeStartPreflight,
  onConfirmStartPreflight,
}: BrowserListDialogsProps) {
  const preflight = startPreflightModal.result
  const requireConfirm = Boolean(preflight?.requireConfirm)
  const canConfirm = !startPreflightModal.loading && !!preflight && (!requireConfirm || startPreflightModal.acknowledged)
  const resultRisk = preflight?.riskLevel || 'unknown'
  const resultRiskBadge = (() => {
    if (resultRisk === 'low') return 'success'
    if (resultRisk === 'medium') return 'warning'
    if (resultRisk === 'high' || resultRisk === 'critical') return 'error'
    return 'info'
  })() as 'success' | 'warning' | 'error' | 'info'

  const renderEntry = (item: BrowserStartPreflightResult['warnings'][number], index: number) => {
    const config = SEVERITY_STYLES[item.severity]
    const Icon = config.icon
    return (
      <div key={item.id || `${item.severity}-${index}`} className="rounded-lg border border-[var(--color-border-default)] bg-[var(--color-bg-secondary)] p-3">
        <div className="flex items-start gap-3">
          <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${item.severity === 'error' ? 'text-[var(--color-error)]' : item.severity === 'warning' ? 'text-[var(--color-warning)]' : 'text-[var(--color-accent)]'}`} />
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={config.badge}>{config.label}</Badge>
              <span className="text-sm text-[var(--color-text-primary)]">{item.message}</span>
            </div>
            {item.detail && <p className="text-xs text-[var(--color-text-muted)] whitespace-pre-line">{item.detail}</p>}
          </div>
        </div>
      </div>
    )
  }

  return (
    <>
      <Modal
        open={proxyErrorModal}
        onClose={onCloseProxyError}
        title="代理链路不可用"
        width="420px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseProxyError} disabled={startingDirect}>取消</Button>
            {pendingStartId && (
              <Button variant="secondary" onClick={onStartDirect} loading={startingDirect}>
                直连启动
              </Button>
            )}
            {pendingStartId && (
              <Link to={`/browser/edit/${pendingStartId}`}>
                <Button onClick={onCloseProxyError} disabled={startingDirect}>去修改代理</Button>
              </Link>
            )}
          </>
        }
      >
        <div className="space-y-3">
          <div className="flex items-start gap-3 p-3 rounded-lg bg-[var(--color-bg-secondary)]">
            <XCircle className="w-5 h-5 text-red-500 mt-0.5 shrink-0" />
            <p className="text-sm text-[var(--color-text-primary)]">{proxyErrorMsg}</p>
          </div>
          <p className="text-sm text-[var(--color-text-muted)]">请前往编辑页面重新选择可用链路；如果是订阅导入，先刷新订阅并确认该节点仍存在。</p>
        </div>
      </Modal>

      {kwModal.profile && (
        <KeywordsModal
          open={kwModal.open}
          profileId={kwModal.profile.profileId}
          profileName={kwModal.profile.profileName}
          initialKeywords={kwModal.profile.keywords || []}
          onClose={onCloseKeywords}
          onSaved={onKeywordsSaved}
        />
      )}

      <Modal
        open={expandModalOpen}
        onClose={onCloseExpand}
        title="实例扩容系统"
        width="480px"
        footer={<Button variant="secondary" onClick={onCloseExpand}>关闭</Button>}
      >
        <div className="space-y-4">
          <div className="bg-[var(--color-bg-secondary)] p-4 rounded-lg flex items-center justify-between border border-[var(--color-border-default)]">
            <div>
              <p className="text-sm font-medium text-[var(--color-text-primary)]">当前使用情况</p>
              <p className="text-xs text-[var(--color-text-muted)] mt-1">每个配置都需要消耗 1 个实例额度</p>
            </div>
            <div className="text-right">
              <span className={`text-2xl font-semibold ${profilesCount >= maxProfileLimit ? 'text-red-500' : 'text-[var(--color-success)]'}`}>
                {profilesCount}
              </span>
              <span className="text-sm text-[var(--color-text-muted)] ml-1">/ {maxProfileLimit}</span>
            </div>
          </div>

          <div className="pt-2 border-t border-[var(--color-border-muted)]">
            <label className="block text-sm font-medium text-[var(--color-text-primary)] mb-2">使用兑换码扩容</label>
            <div className="flex gap-2">
              <Input
                value={cdKey}
                onChange={e => onCdKeyChange(e.target.value)}
                placeholder="输入兑换码 (如 ANT-...)"
                onKeyDown={e => e.key === 'Enter' && onRedeem()}
                className="flex-1"
              />
              <Button onClick={onRedeem} loading={redeeming} disabled={!cdKey.trim()}>
                进行兑换
              </Button>
            </div>
          </div>

          <div className="mt-4 p-3 bg-blue-500/10 border border-blue-500/20 rounded-lg">
            <div className="flex items-center justify-between gap-4">
              <p className="text-sm text-[var(--color-text-primary)]">点亮 GitHub Star 后，可再获赠 50 个永久额度</p>
              <button
                type="button"
                className="shrink-0 rounded-full p-2 text-[var(--color-accent)] transition-colors hover:bg-[var(--color-accent)]/10 disabled:opacity-50"
                onClick={onOpenGithubStarGift}
                disabled={redeeming}
                title="打开 GitHub 并领取赠送"
                aria-label="打开 GitHub 并领取赠送"
              >
                <ExternalLink className="w-4 h-4" />
              </button>
            </div>
          </div>
        </div>
      </Modal>

      <Modal
        open={copyModal.open}
        onClose={onCloseCopy}
        title="复制实例"
        width="420px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseCopy}>取消</Button>
            <Button onClick={onConfirmCopy} loading={copying}>确认复制</Button>
          </>
        }
      >
        <div className="space-y-4">
          <p className="text-sm text-[var(--color-text-muted)]">
            复制实例将保留原有的代理、内核、启动参数、标签等配置，但会生成新的指纹种子。
          </p>
          <FormItem label="新实例名称" required>
            <Input
              value={copyName}
              onChange={e => onCopyNameChange(e.target.value)}
              placeholder="请输入新实例名称"
              autoFocus
            />
          </FormItem>
        </div>
      </Modal>

      <Modal
        open={!!opError}
        onClose={onCloseOpError}
        title="操作失败"
        width="420px"
        footer={<Button onClick={onCloseOpError}>知道了</Button>}
      >
        <div className="text-[var(--color-text-secondary)] whitespace-pre-line">{opError}</div>
      </Modal>

      <Modal
        open={startPreflightModal.open}
        onClose={onCloseStartPreflight}
        title="启动前 preflight 确认"
        width="640px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseStartPreflight} disabled={startPreflightModal.loading}>取消</Button>
            <Button onClick={onConfirmStartPreflight} loading={startPreflightModal.loading} disabled={!canConfirm}>继续启动</Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="rounded-xl border border-[var(--color-border-default)] bg-[var(--color-bg-secondary)] p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h4 className="text-base font-semibold text-[var(--color-text-primary)]">
                    {preflight?.profileName || startPreflightModal.profile?.profileName || '实例'}
                  </h4>
                  <Badge variant={resultRiskBadge}>风险等级：{resultRisk}</Badge>
                  <Badge variant={startPreflightModal.mode === 'direct' ? 'warning' : 'info'}>
                    {startPreflightModal.mode === 'direct' ? '直连启动' : '标准启动'}
                  </Badge>
                </div>
                <p className="mt-2 text-sm text-[var(--color-text-secondary)] whitespace-pre-line">
                  {startPreflightModal.loading ? '正在获取启动前检查结果...' : (preflight?.summary || '暂无 preflight 说明')}
                </p>
              </div>
              <div className="rounded-full bg-[var(--color-bg-surface)] p-2 text-[var(--color-accent)] shadow-sm">
                {resultRisk === 'low' ? <ShieldCheck className="h-5 w-5" /> : <ShieldAlert className="h-5 w-5" />}
              </div>
            </div>
          </div>

          {!startPreflightModal.loading && preflight && (
            <>
              <Alert
                type={preflight.requireConfirm ? 'warning' : 'info'}
                title={preflight.requireConfirm ? '需要显式确认' : '可直接继续'}
                message={(
                  <div className="space-y-2">
                    <p className="whitespace-pre-line">{preflight.summary}</p>
                    <p className="text-xs text-[var(--color-text-muted)]">
                      来源：{preflight.source} · 结果会在启动前用于风险提示，不会修改后端 preflight 或代理推荐逻辑。
                    </p>
                  </div>
                )}
              />

              {preflight.warnings.length > 0 && (
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <AlertTriangle className="h-4 w-4 text-[var(--color-warning)]" />
                    <h5 className="text-sm font-medium text-[var(--color-text-primary)]">风险提示</h5>
                  </div>
                  <div className="space-y-2">
                    {preflight.warnings.map(renderEntry)}
                  </div>
                </div>
              )}

              {preflight.explicitAlerts.length > 0 && (
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <ShieldAlert className="h-4 w-4 text-[var(--color-error)]" />
                    <h5 className="text-sm font-medium text-[var(--color-text-primary)]">明确告警</h5>
                  </div>
                  <div className="space-y-2">
                    {preflight.explicitAlerts.map(renderEntry)}
                  </div>
                </div>
              )}

              {requireConfirm && (
                <div className="rounded-xl border border-[var(--color-error)]/25 bg-[var(--color-error)]/10 p-4">
                  <label className="flex cursor-pointer items-start gap-3">
                    <input
                      type="checkbox"
                      className="mt-1 h-4 w-4 rounded border-[var(--color-border-default)] accent-[var(--color-error)]"
                      checked={startPreflightModal.acknowledged}
                      onChange={(e) => onAcknowledgeStartPreflight(e.target.checked)}
                    />
                    <span className="text-sm text-[var(--color-text-secondary)]">
                      我已阅读上述风险提示，理解在高风险情况下继续启动的后果，并确认要继续。
                    </span>
                  </label>
                </div>
              )}
            </>
          )}

          {startPreflightModal.loading && (
            <div className="rounded-xl border border-[var(--color-border-default)] bg-[var(--color-bg-secondary)] p-4 text-sm text-[var(--color-text-muted)]">
              后端正在生成 preflight 结构化结果，请稍候。
            </div>
          )}
        </div>
      </Modal>
    </>
  )
}
