<template>
  <div v-loading="pageLoading" class="detail-page">
    <header v-if="center" class="detail-header">
      <el-button link type="primary" :icon="ArrowLeft" @click="router.push('/alerts')">返回</el-button>
      <div class="center-summary">
        <div class="center-name">
          <template v-if="nameEditing">
            <el-input
              ref="nameInputRef"
              v-model="nameDraft"
              class="center-name-input"
              size="small"
              maxlength="128"
              aria-label="故障中心名称"
              @keyup.enter="saveCenterName"
              @keyup.esc="cancelNameEdit"
            />
            <el-tooltip content="确认" placement="top">
              <el-button text circle class="name-action is-confirm" :icon="Check" :loading="nameSaving" aria-label="保存故障中心名称" @click="saveCenterName" />
            </el-tooltip>
            <el-tooltip content="取消" placement="top">
              <el-button text circle class="name-action is-cancel" :icon="Close" :disabled="nameSaving" aria-label="取消修改故障中心名称" @click="cancelNameEdit" />
            </el-tooltip>
          </template>
          <template v-else>
            <p>{{ center.name }}</p>
            <el-tooltip content="编辑名称" placement="top">
              <el-button text circle :icon="Edit" aria-label="编辑故障中心名称" @click="startNameEdit" />
            </el-tooltip>
          </template>
        </div>
      </div>
    </header>

    <template v-if="center">
      <main class="workbench">
        <el-tabs v-model="activeTab" class="center-tabs" @tab-change="handleTabChange">
          <el-tab-pane label="活跃告警" name="active">
            <AlertTable :rows="alerts" :loading="alertLoading" :total="total" :page="page" :page-size="pageSize" allow-silence advanced-filters @search="searchAlerts" @page-change="changePage" @page-size-change="changePageSize" @show-detail="showAlertDetail" @silence="openSilenceDrawer" />
          </el-tab-pane>
          <el-tab-pane label="历史告警" name="history">
            <AlertTable :rows="alerts" :loading="alertLoading" :total="total" :page="page" :page-size="pageSize" advanced-filters history-filters @search="searchAlerts" @page-change="changePage" @page-size-change="changePageSize" @show-detail="showAlertDetail" />
          </el-tab-pane>
          <el-tab-pane label="告警静默" name="silence">
            <section class="rule-section">
              <div class="strategy-toolbar">
                <div><el-input v-model="silenceKeyword" clearable aria-label="搜索告警静默" placeholder="搜索策略名称" @keyup.enter="searchSilences" @clear="searchSilences"><template #append><el-button aria-label="搜索" @click="searchSilences"><el-icon><Search /></el-icon></el-button></template></el-input></div>
                <div><el-button :icon="Refresh" @click="searchSilences">刷新</el-button><el-button type="primary" :icon="Plus" @click="openSilenceDrawer()">创建</el-button></div>
              </div>
              <el-table :data="pagedSilenceRules" v-loading="silenceLoading">
                <el-table-column prop="name" label="策略名称" min-width="220" show-overflow-tooltip />
                <el-table-column label="静默类型" width="130"><template #default="{ row }"><el-tag effect="light">{{ row.silence_type === 'weekly' ? '周期性静默' : '一次性静默' }}</el-tag></template></el-table-column>
                <el-table-column label="静默计划" min-width="230" show-overflow-tooltip><template #default="{ row }">{{ silenceScheduleText(row) }}</template></el-table-column>
                <el-table-column label="当前状态" width="110"><template #default="{ row }"><el-tag :type="silenceState(row).type" effect="light">{{ silenceState(row).text }}</el-tag></template></el-table-column>
                <el-table-column label="编辑时间" width="180"><template #default="{ row }">{{ formatTime(row.updated_at) }}</template></el-table-column>
                <el-table-column label="操作" width="90" align="center" fixed="right">
                  <template #default="{ row }">
                    <div class="table-icon-actions">
                      <el-tooltip content="编辑" placement="top"><el-button link type="primary" size="small" :icon="Edit" aria-label="编辑静默策略" @click="openSilenceDrawer(row)" /></el-tooltip>
                      <el-tooltip content="删除" placement="top"><el-button link type="danger" size="small" :icon="Delete" aria-label="删除静默策略" @click="removeSilenceRule(row)" /></el-tooltip>
                    </div>
                  </template>
                </el-table-column>
                <template #empty><TableEmpty /></template>
              </el-table>
              <div class="pagination"><el-pagination v-model:current-page="silencePage" v-model:page-size="silencePageSize" :page-sizes="[10, 20, 50]" :total="filteredSilenceRules.length" layout="total, sizes, prev, pager, next" @size-change="silencePage = 1" /></div>
            </section>
          </el-tab-pane>
          <el-tab-pane label="告警抑制" name="inhibition">
            <section class="rule-section">
              <div class="strategy-toolbar">
                <div><el-input v-model="inhibitionKeyword" clearable aria-label="搜索告警抑制" placeholder="搜索策略名称" @keyup.enter="searchInhibitions" @clear="searchInhibitions"><template #append><el-button aria-label="搜索" @click="searchInhibitions"><el-icon><Search /></el-icon></el-button></template></el-input></div>
                <div><el-button :icon="Refresh" @click="reloadRules">刷新</el-button><el-button type="primary" :icon="Plus" @click="openRuleDrawer()">创建</el-button></div>
              </div>
              <el-table :data="pagedRules">
                <el-table-column prop="name" label="抑制规则名称" min-width="180" show-overflow-tooltip />
                <el-table-column label="源标签" min-width="200"><template #default="{ row }"><code>{{ matcherSummary(row.source_matchers) }}</code></template></el-table-column>
                <el-table-column label="目标标签" min-width="200"><template #default="{ row }"><code>{{ matcherSummary(row.target_matchers) }}</code></template></el-table-column>
                <el-table-column label="相等标签" min-width="180"><template #default="{ row }"><code>{{ selectorSummary(row.equal_labels) }}</code></template></el-table-column>
                <el-table-column label="启用状态" width="100"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" effect="light">{{ row.status === 1 ? '已启用' : '已停用' }}</el-tag></template></el-table-column>
                <el-table-column label="编辑时间" width="180"><template #default="{ row }">{{ formatTime(row.updated_at) }}</template></el-table-column>
                <el-table-column label="操作" width="90" align="center" fixed="right">
                  <template #default="{ row }">
                    <div class="table-icon-actions">
                      <el-tooltip content="编辑" placement="top"><el-button link type="primary" size="small" :icon="Edit" aria-label="编辑抑制规则" @click="openRuleDrawer(row)" /></el-tooltip>
                      <el-tooltip content="删除" placement="top"><el-button link type="danger" size="small" :icon="Delete" aria-label="删除抑制规则" @click="removeRule(row)" /></el-tooltip>
                    </div>
                  </template>
                </el-table-column>
                <template #empty><TableEmpty /></template>
              </el-table>
              <div class="pagination"><el-pagination v-model:current-page="inhibitionPage" v-model:page-size="inhibitionPageSize" :page-sizes="[10, 20, 50]" :total="filteredRules.length" layout="total, sizes, prev, pager, next" @size-change="inhibitionPage = 1" /></div>
            </section>
          </el-tab-pane>
          <el-tab-pane label="分组收敛" name="grouping">
            <section class="rule-section grouping-section">
              <div class="strategy-toolbar">
                <div><el-input v-model="groupingKeyword" clearable aria-label="搜索分组收敛" placeholder="搜索策略名称" @keyup.enter="searchGroupings" @clear="searchGroupings"><template #append><el-button aria-label="搜索" @click="searchGroupings"><el-icon><Search /></el-icon></el-button></template></el-input></div>
                <div><el-button :icon="Refresh" @click="loadGroupingRules">刷新</el-button><el-button type="primary" :icon="Plus" @click="openGroupingDrawer()">创建</el-button></div>
              </div>
              <el-table v-loading="groupingLoading" :data="pagedGroupingRows" table-layout="fixed">
                <el-table-column prop="name" label="策略名称" min-width="150" show-overflow-tooltip />
                <el-table-column label="匹配告警" min-width="190">
                  <template #default="{ row }"><code>{{ row.matchers.length ? matcherSummary(row.matchers) : '全部告警' }}</code></template>
                </el-table-column>
                <el-table-column label="分组标签" min-width="190">
                  <template #default="{ row }">
                    <div v-if="row.group_by.length" class="group-labels"><el-tag v-for="selector in row.group_by" :key="`${selector.namespace}:${selector.key}`" size="small" effect="light">{{ labelNamespaceText(selector.namespace) }} · {{ selector.key }}</el-tag></div>
                    <span v-else class="all-in-one-group">合并为一个组</span>
                  </template>
                </el-table-column>
                <el-table-column prop="wait_time" label="等待时间" width="100" />
                <el-table-column prop="interval_time" label="间隔时间" width="110" />
                <el-table-column prop="priority" label="优先级" width="86" align="center" />
                <el-table-column label="启用状态" width="100"><template #default="{ row }"><el-tag :type="row.enabled ? 'success' : 'info'" effect="light">{{ row.enabled ? '已启用' : '已停用' }}</el-tag></template></el-table-column>
                <el-table-column label="编辑时间" width="180"><template #default="{ row }">{{ formatTime(row.updated_at) }}</template></el-table-column>
                <el-table-column label="操作" width="90" align="center" fixed="right">
                  <template #default="{ row }">
                    <div class="table-icon-actions">
                      <el-tooltip content="编辑" placement="top"><el-button link type="primary" size="small" :icon="Edit" aria-label="编辑分组策略" @click="openGroupingDrawer(row)" /></el-tooltip>
                      <el-tooltip content="删除" placement="top"><el-button link type="danger" size="small" :icon="Delete" aria-label="删除分组策略" @click="removeGroupingPolicy(row)" /></el-tooltip>
                    </div>
                  </template>
                </el-table-column>
                <template #empty><TableEmpty /></template>
              </el-table>
              <div class="pagination"><el-pagination v-model:current-page="groupingPage" v-model:page-size="groupingPageSize" :page-sizes="[10, 20, 50]" :total="filteredGroupingRows.length" layout="total, sizes, prev, pager, next" @size-change="groupingPage = 1" /></div>
            </section>
          </el-tab-pane>
          <el-tab-pane label="自动化处置" name="automation">
            <AlertActionPanel :center-id="center.id" />
          </el-tab-pane>
          <el-tab-pane label="通知配置" name="notification">
            <section class="notification-section">
              <div class="notification-heading">
                <div>
                  <strong><el-icon><Bell /></el-icon>通知配置</strong>
                </div>
                <div class="notification-actions">
                  <el-button v-if="!notificationEditing" type="primary" :icon="Edit" @click="notificationEditing = true">编辑</el-button>
                  <template v-else><el-button @click="cancelNotificationEdit">取消</el-button><el-button type="primary" :loading="savingNotification" @click="saveNotificationConfig">保存</el-button></template>
                </div>
              </div>

              <div class="notification-grid">
                <article class="notification-card">
                  <h3><el-icon><Setting /></el-icon>基本配置</h3>
                  <el-form label-position="top" class="notification-form">
                    <el-form-item>
                      <template #label><span class="form-label">默认通知对象<el-tooltip content="未命中告警路由时，将告警发送给这里配置的通知对象。" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
                      <el-select v-model="notificationConfig.default_channel_ids" multiple filterable collapse-tags :max-collapse-tags="2" :disabled="!notificationEditing" placeholder="请选择通知对象">
                        <el-option v-for="channel in enabledNotificationChannels" :key="channel.id" :label="channel.name" :value="channel.id"><span>{{ channel.name }}</span><small>{{ channel.type }}</small></el-option>
                      </el-select>
                    </el-form-item>
                    <el-form-item required>
                      <template #label><span class="form-label">重复通知间隔<el-tooltip content="持续告警再次发送通知的最小间隔，默认 60 分钟。" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
                      <div class="number-field"><el-input-number v-model="notificationConfig.repeat_interval_minutes" :min="1" :max="10080" :disabled="!notificationEditing" controls-position="right" /><span>分钟</span></div>
                    </el-form-item>
                    <el-form-item>
                      <template #label><span class="form-label">启用恢复通知<el-tooltip content="启用后，告警状态恢复时会通知对应对象。" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
                      <el-switch v-model="notificationConfig.recovery_notification" :disabled="!notificationEditing" />
                    </el-form-item>
                  </el-form>
                </article>

                <article class="notification-card route-card">
                  <h3><el-icon><PriceTag /></el-icon>告警路由<el-tooltip content="根据标签将告警发送给特定通知对象；按照从上到下的顺序匹配，优先级高于默认通知对象。" placement="top"><el-icon class="notification-help-icon" tabindex="0" aria-label="告警路由说明"><QuestionFilled /></el-icon></el-tooltip></h3>
                  <div class="route-table-head"><span>标签来源</span><span>标签 Key</span><span>标签 Value</span><span>通知对象</span><span>操作</span></div>
                  <div v-if="notificationConfig.routes.length" class="route-list">
                    <div v-for="(item, index) in notificationConfig.routes" :key="item.id || index" class="route-row">
                      <el-select v-model="item.label_namespace" :disabled="!notificationEditing" placeholder="标签来源" @change="item.label_key = ''"><el-option v-for="option in labelNamespaceOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select>
                      <el-select v-if="item.label_namespace === 'system_labels'" v-model="item.label_key" :disabled="!notificationEditing" placeholder="系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="item.label_key" :disabled="!notificationEditing" maxlength="64" placeholder="标签 Key" />
                      <el-input v-model="item.label_value" :disabled="!notificationEditing" placeholder="标签 Value" />
                      <el-select v-model="item.channel_ids" multiple filterable collapse-tags :disabled="!notificationEditing" placeholder="通知对象"><el-option v-for="channel in enabledNotificationChannels" :key="channel.id" :label="channel.name" :value="channel.id" /></el-select>
                      <el-button v-if="notificationEditing" text type="danger" :icon="Delete" aria-label="删除告警路由" @click="removeNotificationRoute(index)" />
                      <span v-else>—</span>
                    </div>
                  </div>
                  <el-empty v-else description="暂无告警路由" :image-size="64" />
                  <el-button v-if="notificationEditing" class="add-route" plain :icon="Plus" @click="addNotificationRoute">添加告警路由</el-button>
                </article>
              </div>
            </section>
          </el-tab-pane>
        </el-tabs>
      </main>
    </template>

    <el-empty v-else-if="!pageLoading" description="故障中心不存在"><el-button type="primary" @click="router.push('/alerts')">返回故障中心</el-button></el-empty>

    <el-drawer v-model="detailVisible" class="center-drawer" size="min(460px, 94vw)" :close-on-click-modal="false">
      <template #header><div class="drawer-header"><DrawerBackButton @click="detailVisible = false" /><span class="drawer-title">告警详情</span></div></template>
      <div v-if="currentAlert" class="alert-detail">
        <dl><div class="detail-item--full"><dt>告警名称</dt><dd><router-link v-if="canOpenAlertRule(currentAlert)" class="detail-link" :to="alertRuleEditRoute(currentAlert)">{{ alertResult.alertname || currentAlert.title || '—' }}</router-link><template v-else>{{ alertResult.alertname || currentAlert.title || '—' }}</template></dd></div><div><dt>告警等级</dt><dd><el-tag class="severity-tag" :type="severityType(currentAlert.severity)" effect="light" size="small">{{ currentAlert.severity || '—' }}</el-tag></dd></div><div><dt>告警状态</dt><dd><el-tag :type="statusType(currentAlert.status)" effect="light" size="small">{{ displayStatus(currentAlert) }}</el-tag></dd></div><div v-if="isPrometheusAlert(currentAlert)"><dt>告警规则组</dt><dd>{{ currentAlert.rule_group_name || '—' }}</dd></div><div><dt>告警来源</dt><dd>{{ alertSourceText(currentAlert) }}</dd></div><div v-if="isPrometheusAlert(currentAlert)"><dt>数据源</dt><dd>{{ alertResult.datasource || '—' }}</dd></div><div v-if="isPrometheusAlert(currentAlert)"><dt>当前值</dt><dd>{{ alertResult.value || '—' }}</dd></div><div v-if="isPrometheusAlert(currentAlert)" class="detail-item--full"><dt>命中条件</dt><dd>{{ alertResult.condition || '—' }}</dd></div><div v-if="isPrometheusAlert(currentAlert)" class="detail-item--full"><dt>告警指纹</dt><dd class="detail-monospace">{{ alertResult.fingerprint || currentAlert.fingerprint || '—' }}</dd></div><div v-if="isPrometheusAlert(currentAlert)" class="detail-item--full"><dt>PromQL</dt><dd class="detail-monospace">{{ alertResult.expression || '—' }}</dd></div><div><dt>首次发现</dt><dd>{{ formatTime(currentAlert.started_at) }}</dd></div><div><dt>正式触发</dt><dd>{{ formatTime(currentAlert.fired_at) }}</dd></div><div><dt>最近事件</dt><dd>{{ formatTime(currentAlert.last_event_at) }}</dd></div><div v-if="currentAlert.resolved_at"><dt>{{ currentAlert.status === 'terminated' ? '终止时间' : '恢复时间' }}</dt><dd>{{ formatTime(currentAlert.resolved_at) }}</dd></div><div v-if="isSilenced(currentAlert)" class="detail-item--full"><dt>静默至</dt><dd>{{ formatTime(currentAlert.silenced_until) }}</dd></div><div v-if="alertResult.annotations?.description || currentAlert.rule_description" class="detail-item--full"><dt>描述信息</dt><dd>{{ alertResult.annotations?.description || currentAlert.rule_description }}</dd></div><div class="detail-item--full"><dt>通知状态</dt><dd class="notification-state-detail"><span>{{ notificationStatusText(currentAlert) }}</span><small v-if="showNotificationReason(currentAlert)">{{ currentAlert.notification_reason }}</small><small v-if="hasUpcomingNotification(currentAlert)">预计通知：{{ formatTime(currentAlert.next_notification_at) }}</small></dd></div></dl>
        <section v-if="isPrometheusAlert(currentAlert)" class="alert-label-sections">
          <div v-for="section in alertLabelSections" :key="section.key" class="alert-label-section">
            <span>{{ section.title }}</span>
            <div class="scope-tags"><span v-for="item in section.items" :key="item.key" class="scope-tag" :class="labelToneClass(item.key)">{{ item.key }}:{{ item.value }}</span><small v-if="!section.items.length">暂无标签</small></div>
          </div>
        </section>
        <section class="alert-timeline">
          <button type="button" class="timeline-toggle" :aria-expanded="lifecycleExpanded" @click="toggleLifecycle">
            <span>生命周期</span>
            <span class="timeline-toggle-meta"><small v-if="eventLoaded">{{ alertEvents.length }} 条记录</small><span>{{ lifecycleExpanded ? '收起' : '展开' }}</span><el-icon :class="{ expanded: lifecycleExpanded }"><ArrowDown /></el-icon></span>
          </button>
          <div v-show="lifecycleExpanded" class="timeline-content">
            <el-timeline v-loading="eventLoading">
              <el-timeline-item v-for="item in alertEvents" :key="item.id" :timestamp="formatTime(item.created_at)" :type="eventTimelineType(item)" placement="top">
                <div class="timeline-event-title">{{ eventTitle(item) }}</div>
                <div v-if="item.reason" class="timeline-event-reason">{{ eventReason(item.reason) }}</div>
              </el-timeline-item>
            </el-timeline>
            <el-empty v-if="eventLoaded && !eventLoading && !alertEvents.length" description="暂无生命周期记录" :image-size="48" />
          </div>
        </section>
        <section class="alert-result">
          <div class="detail-section-heading"><span>最近一次结果</span></div>
          <pre>{{ formatResult(currentAlert.last_result) }}</pre>
        </section>
      </div>
    </el-drawer>

    <el-drawer v-model="ruleDrawerVisible" class="center-drawer config-drawer" size="min(460px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header><div class="drawer-header"><DrawerBackButton @click="ruleDrawerVisible = false" /><span class="drawer-title">{{ ruleForm.id ? '编辑抑制规则' : '创建抑制规则' }}</span></div></template>
      <el-form ref="ruleFormRef" :model="ruleForm" :rules="ruleRules" label-position="top" class="strategy-form inhibition-form">
        <el-form-item label="抑制规则名称" prop="name">
          <el-input v-model="ruleForm.name" maxlength="128" show-word-limit aria-label="抑制规则名称" />
        </el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">源标签<el-tooltip content="匹配源告警的标签，多组条件之间是“且”关系" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(matcher, index) in ruleForm.source_matchers" :key="index" class="label-match-item">
              <el-button v-if="ruleForm.source_matchers.length > 1" class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除源标签条件 ${index + 1}`" @click="removeRuleMatcher('source_matchers', index)" />
              <div class="label-source-control">
                <span>标签来源</span>
                <el-radio-group v-model="matcher.namespace" class="label-source-options" aria-label="源标签来源" @change="matcher.key = ''"><el-radio-button v-for="option in labelNamespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group>
              </div>
              <div class="label-match-fields"><div class="label-field-control"><span>标签 Key</span><el-select v-if="matcher.namespace === 'system_labels'" v-model="matcher.key" aria-label="源系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="matcher.key" maxlength="64" aria-label="源标签 Key" /></div><div class="label-field-control"><span>标签 Value</span><el-input v-model="matcher.value" maxlength="255" aria-label="源标签 Value" /></div></div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="ruleForm.source_matchers.length >= 10" @click="addRuleMatcher('source_matchers')">添加源标签条件</el-button>
          </div>
        </el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">目标标签<el-tooltip content="匹配目标告警的标签，多组条件之间是“且”关系" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(matcher, index) in ruleForm.target_matchers" :key="index" class="label-match-item">
              <el-button v-if="ruleForm.target_matchers.length > 1" class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除目标标签条件 ${index + 1}`" @click="removeRuleMatcher('target_matchers', index)" />
              <div class="label-source-control">
                <span>标签来源</span>
                <el-radio-group v-model="matcher.namespace" class="label-source-options" aria-label="目标标签来源" @change="matcher.key = ''"><el-radio-button v-for="option in labelNamespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group>
              </div>
              <div class="label-match-fields"><div class="label-field-control"><span>标签 Key</span><el-select v-if="matcher.namespace === 'system_labels'" v-model="matcher.key" aria-label="目标系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="matcher.key" maxlength="64" aria-label="目标标签 Key" /></div><div class="label-field-control"><span>标签 Value</span><el-input v-model="matcher.value" maxlength="255" aria-label="目标标签 Value" /></div></div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="ruleForm.target_matchers.length >= 10" @click="addRuleMatcher('target_matchers')">添加目标标签条件</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <template #label><span class="form-label">相等标签<el-tooltip content="可选；源告警与目标告警在指定来源下的标签值必须相等" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(selector, index) in ruleForm.equal_labels" :key="index" class="label-match-item equal-label-item">
              <el-button class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除相等标签 ${index + 1}`" @click="removeEqualLabel(index)" />
              <div class="label-source-control"><span>标签来源</span><el-radio-group v-model="selector.namespace" class="label-source-options" aria-label="相等标签来源" @change="selector.key = ''"><el-radio-button v-for="option in labelNamespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group></div>
              <div class="label-field-control equal-label-key"><span>标签 Key</span><el-select v-if="selector.namespace === 'system_labels'" v-model="selector.key" aria-label="相等系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="selector.key" maxlength="64" aria-label="相等标签 Key" /></div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="ruleForm.equal_labels.length >= 10" @click="addEqualLabel">添加相等标签</el-button>
          </div>
        </el-form-item>
        <el-form-item label="启用状态"><el-switch v-model="ruleForm.enabled" /></el-form-item>
      </el-form>
      <template #footer><div class="drawer-footer"><el-button @click="ruleDrawerVisible = false">取消</el-button><el-button type="primary" :loading="savingRule" @click="saveRule">保存</el-button></div></template>
    </el-drawer>

    <el-drawer v-model="groupingDrawerVisible" class="center-drawer config-drawer" size="min(460px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header><div class="drawer-header"><DrawerBackButton @click="groupingDrawerVisible = false" /><span class="drawer-title">{{ grouping.id ? '编辑分组收敛策略' : '创建分组收敛策略' }}</span></div></template>
      <el-form label-position="top" class="strategy-form grouping-form">
        <el-form-item label="策略名称" required>
          <el-input v-model="grouping.name" maxlength="128" show-word-limit aria-label="分组收敛策略名称" />
        </el-form-item>
        <el-form-item>
          <template #label><span class="form-label">匹配告警<el-tooltip content="只有满足全部匹配条件的告警才会进入该策略；不配置表示匹配全部告警。策略按优先级从小到大匹配，只使用第一条命中的策略" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(matcher, index) in grouping.matchers" :key="index" class="label-match-item">
              <el-button class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除匹配条件 ${index + 1}`" @click="removeGroupingMatcher(index)" />
              <div class="label-source-control"><span>标签来源</span><el-radio-group v-model="matcher.namespace" class="label-source-options" aria-label="匹配标签来源" @change="matcher.key = ''"><el-radio-button v-for="option in labelNamespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group></div>
              <div class="label-match-fields"><div class="label-field-control"><span>标签 Key</span><el-select v-if="matcher.namespace === 'system_labels'" v-model="matcher.key" aria-label="匹配系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="matcher.key" maxlength="64" aria-label="匹配标签 Key" /></div><div class="label-field-control"><span>标签 Value</span><el-input v-model="matcher.value" maxlength="255" aria-label="匹配标签 Value" /></div></div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="grouping.matchers.length >= 10" @click="addGroupingMatcher">添加匹配条件</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <template #label><span class="form-label">分组标签<el-tooltip content="系统读取所选标签的实际值进行分组；相同值的告警合并为一条通知。不配置时，命中策略的所有告警合并为一个大组" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(selector, index) in grouping.group_by" :key="index" class="label-match-item grouping-label-item">
              <el-button class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除分组标签 ${index + 1}`" @click="removeGroupingSelector(index)" />
              <div class="label-source-control"><span>标签来源</span><el-radio-group v-model="selector.namespace" class="label-source-options" aria-label="分组标签来源" @change="selector.key = ''"><el-radio-button v-for="option in labelNamespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group></div>
              <div class="label-field-control grouping-label-key"><span>标签 Key</span><el-select v-if="selector.namespace === 'system_labels'" v-model="selector.key" aria-label="分组系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="selector.key" maxlength="64" aria-label="分组标签 Key" /></div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="grouping.group_by.length >= 10" @click="addGroupingSelector">添加分组标签</el-button>
          </div>
        </el-form-item>
        <div class="grouping-timing-grid">
          <el-form-item>
            <template #label><span class="form-label">等待时间<el-tooltip content="从同组第一条告警开始计时，用于收集同组告警；期间恢复的告警不会通知，但会记录为“未通知已恢复”，P0 告警立即发送" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
            <div class="number-field"><el-input-number v-model="grouping.wait_seconds" :min="0" :max="86400" :step="30" controls-position="right" /><span>秒</span></div>
          </el-form-item>
          <el-form-item>
            <template #label><span class="form-label">间隔时间<el-tooltip content="同一分组发送新告警通知的最小间隔，用于避免通知过于频繁，默认 5 分钟" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
            <div class="number-field"><el-input-number v-model="grouping.interval_seconds" :min="60" :max="86400" :step="300" controls-position="right" /><span>秒</span></div>
          </el-form-item>
        </div>
        <el-form-item>
          <template #label><span class="form-label">优先级<el-tooltip content="数值越小越先匹配；一条告警只进入第一条命中的启用策略" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-input-number v-model="grouping.priority" :min="1" :max="9999" controls-position="right" class="priority-input" />
        </el-form-item>
        <el-form-item label="启用状态">
          <el-switch v-model="grouping.enabled" aria-label="启用分组收敛" />
        </el-form-item>
      </el-form>
      <template #footer><div class="drawer-footer"><el-button @click="groupingDrawerVisible = false">取消</el-button><el-button type="primary" :loading="savingGrouping" @click="saveGrouping">保存</el-button></div></template>
    </el-drawer>

    <el-drawer v-model="silenceDrawerVisible" class="center-drawer config-drawer" size="min(460px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header><div class="drawer-header"><DrawerBackButton @click="silenceDrawerVisible = false" /><span class="drawer-title">{{ silenceForm.id ? '编辑告警静默' : '创建告警静默' }}</span></div></template>
      <el-form ref="silenceFormRef" :model="silenceForm" :rules="silenceFormRules" label-position="top" class="strategy-form silence-form">
        <el-form-item label="策略名称" prop="name">
          <el-input v-model="silenceForm.name" maxlength="128" show-word-limit aria-label="静默策略名称" />
        </el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">匹配标签<el-tooltip content="从告警已有标签中选择字段并填写匹配值；多组条件之间是“且”关系" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(matcher, index) in silenceForm.matchers" :key="index" class="label-match-item">
              <el-button v-if="silenceForm.matchers.length > 1" class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除匹配条件 ${index + 1}`" title="删除条件" @click="removeSilenceMatcher(index)" />
              <div class="label-source-control">
                <span>标签来源</span>
                <el-radio-group v-model="matcher.namespace" class="label-source-options" aria-label="标签来源" @change="matcher.key = ''">
                  <el-radio-button v-for="option in labelNamespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button>
                </el-radio-group>
              </div>
              <div class="label-match-fields">
                <div class="label-field-control">
                  <span>标签 Key</span>
                  <el-select v-if="matcher.namespace === 'system_labels'" v-model="matcher.key" aria-label="静默系统标签"><el-option v-for="option in silenceSystemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="matcher.key" maxlength="64" aria-label="标签 Key" />
                </div>
                <div class="label-field-control">
                  <span>标签 Value</span>
                  <el-input v-model="matcher.value" maxlength="255" aria-label="标签 Value" />
                </div>
              </div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="silenceForm.matchers.length >= 10" @click="addSilenceMatcher">添加匹配条件</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <template #label><span class="form-label">静默类型<el-tooltip content="一次性静默按起止日期生效；周期性静默按星期和每日时间永久循环" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-radio-group v-model="silenceForm.silence_type" class="silence-type-options" aria-label="静默类型"><el-radio-button value="once">一次性静默</el-radio-button><el-radio-button value="weekly">周期性静默</el-radio-button></el-radio-group>
        </el-form-item>
        <el-form-item v-if="silenceForm.silence_type === 'once'" prop="date_range">
          <template #label><span class="form-label">静默时段<el-tooltip content="到达结束时间后自动失效，告警恢复正常通知；规则会保留为“已过期”" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-date-picker v-model="silenceForm.date_range" type="datetimerange" value-format="YYYY-MM-DDTHH:mm:ssZ" start-placeholder="" end-placeholder="" range-separator="至" aria-label="一次性静默时段" />
        </el-form-item>
        <template v-else>
          <el-form-item prop="weekdays">
            <template #label><span class="form-label">生效星期<el-tooltip content="选择周期性静默每周生效的星期" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
            <el-select v-model="silenceForm.weekdays" multiple clearable aria-label="生效星期" @change="sortSilenceWeekdays"><el-option v-for="item in weekdayOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select>
          </el-form-item>
          <el-form-item prop="daily_time_range">
            <template #label><span class="form-label">每日静默时间<el-tooltip content="按所选星期永久循环，直到删除规则；支持跨天，例如周一 22:00 至 06:00 会静默到周二 06:00" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
            <el-time-picker v-model="silenceForm.daily_time_range" is-range range-separator="至" start-placeholder="" end-placeholder="" format="HH:mm" value-format="HH:mm" aria-label="每日静默时间" />
          </el-form-item>
        </template>
        <el-form-item label="启用状态">
          <el-switch v-model="silenceForm.enabled" aria-label="启用告警静默" />
        </el-form-item>
      </el-form>
      <template #footer><div class="drawer-footer"><el-button @click="silenceDrawerVisible = false">取消</el-button><el-button type="primary" :loading="savingSilence" @click="saveSilenceRule">保存</el-button></div></template>
    </el-drawer>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, defineComponent, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import TableEmpty from '../../components/TableEmpty.vue'
import { ElButton, ElDatePicker, ElIcon, ElInput, ElMessage, ElMessageBox, ElOption, ElPagination, ElSelect, ElTable, ElTableColumn, ElTag } from 'element-plus'
import { ArrowDown, ArrowLeft, Bell, Check, Clock, Close, Delete, Edit, Loading, Plus, PriceTag, QuestionFilled, Refresh, Search, Setting } from '@element-plus/icons-vue'
import { createGroupingRule, createInhibitionRule, createSilenceRule, deleteGroupingRule, deleteInhibitionRule, deleteSilenceRule, getAlertEvents, getAlerts, getFaultCenterNotificationConfig, getFaultCenters, getGroupingRules, getInhibitionRules, getSilenceRules, updateFaultCenter, updateFaultCenterNotificationConfig, updateGroupingRule, updateInhibitionRule, updateSilenceRule } from '../../api/alert'
import { getNotificationChannels } from '../../api/notification'
import AlertActionPanel from './components/AlertActionPanel.vue'

const AlertTable = defineComponent({
  props: { rows: Array, loading: Boolean, total: Number, page: Number, pageSize: Number, allowSilence: Boolean, advancedFilters: Boolean, historyFilters: Boolean },
  emits: ['search', 'page-change', 'page-size-change', 'show-detail', 'silence'],
  setup(props, { emit }) {
    const keyword = ref('')
    const severity = ref(''), dateRange = ref([])
    const clock = ref(Date.now())
    const durationTicker = window.setInterval(() => { clock.value = Date.now() }, 1000)
    let keywordSearchTimer
    onBeforeUnmount(() => {
      window.clearInterval(durationTicker)
      window.clearTimeout(keywordSearchTimer)
    })
    const submit = () => emit('search', props.advancedFilters ? { keyword: keyword.value, severity: severity.value, start_date: dateRange.value?.[0] || '', end_date: dateRange.value?.[1] || '' } : keyword.value)
    const submitImmediately = () => {
      window.clearTimeout(keywordSearchTimer)
      submit()
    }
    const updateKeyword = value => {
      keyword.value = value
      window.clearTimeout(keywordSearchTimer)
      keywordSearchTimer = window.setTimeout(submit, 300)
    }
    const clearFilters = () => {
      window.clearTimeout(keywordSearchTimer)
      keyword.value = ''
      severity.value = ''
      dateRange.value = []
      submit()
    }
    const hasFilters = () => Boolean(keyword.value || severity.value || dateRange.value?.length)
    const select = (model, placeholder, options) => h(ElSelect, { modelValue: model.value, clearable: true, placeholder, style: 'width:150px', 'onUpdate:modelValue': value => { model.value = value || ''; submit() } }, { default: () => options.map(item => h(ElOption, { key: item.value, label: item.label, value: item.value })) })
    return () => h('section', { class: 'alert-section' }, [
      h('div', { class: ['alert-filter', props.advancedFilters && 'is-advanced'] }, [
        h(ElInput, { modelValue: keyword.value, clearable: true, placeholder: '搜索告警名称', prefixIcon: Search, 'onUpdate:modelValue': updateKeyword, onKeyup: event => { if (event.key === 'Enter') submitImmediately() }, onClear: submitImmediately }),
        props.advancedFilters ? select(severity, '告警等级', [{ label: 'P0', value: 'P0' }, { label: 'P1', value: 'P1' }, { label: 'P2', value: 'P2' }]) : null,
        props.historyFilters ? h(ElDatePicker, { class: 'alert-date-filter', modelValue: dateRange.value, type: 'daterange', format: 'MM-DD', valueFormat: 'YYYY-MM-DD', startPlaceholder: '开始日期', endPlaceholder: '结束日期', rangeSeparator: '至', clearable: true, 'onUpdate:modelValue': value => { dateRange.value = value || []; submit() } }) : null,
        h(ElButton, { disabled: !hasFilters(), onClick: clearFilters }, () => '清空'),
        props.advancedFilters ? h(ElButton, { class: 'filter-refresh', icon: Refresh, onClick: submit }, () => '刷新') : null,
      ]),
      h('div', { class: 'table-wrap' }, [h(ElTable, { data: props.rows, tableLayout: 'fixed', onRowClick: row => emit('show-detail', row), style: 'width:100%' }, { default: () => [
        h(ElTableColumn, { prop: 'title', label: '告警名称', minWidth: 180, showOverflowTooltip: true }),
        h(ElTableColumn, { label: '告警等级', width: 105, align: 'center', headerAlign: 'center' }, { default: ({ row }) => h(ElTag, { class: 'severity-tag', type: severityType(row.severity), effect: 'light', size: 'small' }, () => row.severity || '—') }),
        h(ElTableColumn, { label: '告警状态', width: 105, align: 'center', headerAlign: 'center' }, { default: ({ row }) => h(ElTag, { type: statusType(row.status), effect: 'light', size: 'small' }, () => displayStatus(row)) }),
        h(ElTableColumn, { label: '通知状态', width: 130, align: 'center', headerAlign: 'center' }, { default: ({ row }) => h(ElTag, { type: notificationStatusType(effectiveNotificationStatus(row)), effect: 'light', size: 'small', title: row.notification_reason || '' }, () => notificationStatusText(row)) }),
        h(ElTableColumn, { label: props.historyFilters ? '告警时间' : '触发时间', width: 190, className: 'alert-time-column' }, { default: ({ row }) => props.historyFilters
          ? h('div', { class: 'alert-time-pair' }, [
            h('div', { 'aria-label': '触发时间' }, formatTime(alertTriggerTime(row))),
            h('div', { class: { 'is-empty': !row.resolved_at }, 'aria-label': '恢复时间' }, formatTime(row.resolved_at)),
          ])
          : formatTime(alertTriggerTime(row)) }),
        h(ElTableColumn, { label: '持续时长', width: 180, className: 'alert-time-column' }, { default: ({ row }) => {
          const seconds = alertDurationSeconds(row, clock.value)
          const duration = formatAlertDuration(row, clock.value)
          if (seconds === null) return duration
          const filledSegments = durationMeterSegments(seconds)
          return h('div', { class: 'duration-cell', title: `${duration}；进度条以 24 小时为视觉上限` }, [
            h('div', { class: 'duration-value' }, [h(ElIcon, { class: 'duration-icon', 'aria-hidden': 'true' }, () => h(Clock)), h('span', { class: 'duration-text' }, duration)]),
            h('div', { class: 'duration-segments', role: 'progressbar', 'aria-label': `持续时长 ${duration}`, 'aria-valuemin': 0, 'aria-valuemax': 86400, 'aria-valuenow': Math.min(seconds, 86400) }, Array.from({ length: 10 }, (_, index) => h('i', { class: ['duration-segment', { 'is-filled': index < filledSegments }] }))),
          ])
        } }),
        h(ElTableColumn, { label: '操作', width: props.allowSilence ? 120 : 72, fixed: 'right' }, { default: ({ row }) => [h(ElButton, { link: true, type: 'primary', onClick: event => { event.stopPropagation(); emit('show-detail', row) } }, () => '详情'), props.allowSilence ? (isSilenced(row) ? h(ElButton, { link: true, type: 'info', disabled: true }, () => '已静默') : h(ElButton, { link: true, type: 'primary', onClick: event => { event.stopPropagation(); emit('silence', row) } }, () => '静默')) : null] }),
      ], empty: () => h(TableEmpty) }), props.loading ? h('div', { class: 'table-loading' }, [h(ElIcon, { class: 'is-loading', size: 24 }, () => h(Loading))]) : null]),
      h('div', { class: 'pagination' }, [h(ElPagination, { currentPage: props.page, pageSize: props.pageSize, pageSizes: [10, 20, 50], total: props.total, layout: 'total, sizes, prev, pager, next', 'onUpdate:currentPage': value => emit('page-change', value), 'onUpdate:pageSize': value => emit('page-size-change', value) })]),
    ])
  },
})

const route = useRoute(), router = useRouter()
const pageLoading = ref(false), alertLoading = ref(false), groupingLoading = ref(false), silenceLoading = ref(false), savingGrouping = ref(false), savingRule = ref(false), savingSilence = ref(false), savingNotification = ref(false)
const nameEditing = ref(false), nameDraft = ref(''), nameSaving = ref(false), nameInputRef = ref()
const center = ref(null), alerts = ref([]), groupingPolicies = ref([]), rules = ref([]), silencePolicies = ref([]), notificationChannels = ref([])
const activeTab = ref('active'), keyword = ref(''), page = ref(1), pageSize = ref(10), total = ref(0)
const silencePage = ref(1), silencePageSize = ref(10)
const inhibitionPage = ref(1), inhibitionPageSize = ref(10)
const groupingPage = ref(1), groupingPageSize = ref(10)
const alertFilters = reactive({ severity: '', alert_status: '', start_date: '', end_date: '' })
const groupingKeyword = ref(''), inhibitionKeyword = ref(''), silenceKeyword = ref('')
const detailVisible = ref(false), currentAlert = ref(null), alertEvents = ref([]), eventLoading = ref(false), eventLoaded = ref(false), lifecycleExpanded = ref(false), groupingDrawerVisible = ref(false), ruleDrawerVisible = ref(false), silenceDrawerVisible = ref(false)
const notificationEditing = ref(false), notificationSnapshot = ref(null)
const ruleFormRef = ref(), silenceFormRef = ref()
const grouping = reactive({ id: null, name: '', matchers: [], group_by: [], wait_seconds: 30, interval_seconds: 300, priority: 100, enabled: true })
const emptyMatcher = () => ({ namespace: 'labels', key: '', value: '' })
const emptySelector = () => ({ namespace: 'labels', key: '' })
const labelNamespaceOptions = [{ label: '系统标签', value: 'system_labels' }, { label: 'PromQL 标签', value: 'labels' }, { label: '数据源标签', value: 'datasource_labels' }, { label: '额外标签', value: 'external_labels' }]
const systemLabelOptions = [{ label: 'alertname（告警名称）', value: 'alertname' }, { label: 'severity（告警等级）', value: 'severity' }, { label: 'datasource（数据源）', value: 'datasource' }]
const silenceSystemLabelOptions = systemLabelOptions.filter(option => option.value !== 'severity')
const weekdayOptions = [{ label: '周一', value: 1 }, { label: '周二', value: 2 }, { label: '周三', value: 3 }, { label: '周四', value: 4 }, { label: '周五', value: 5 }, { label: '周六', value: 6 }, { label: '周日', value: 7 }]
const ruleForm = reactive({ id: null, name: '', source_matchers: [emptyMatcher()], target_matchers: [emptyMatcher()], equal_labels: [], enabled: true })
const silenceForm = reactive({ id: null, name: '', matchers: [emptyMatcher()], silence_type: 'once', date_range: [], weekdays: [], daily_time_range: [], enabled: true })
const silenceClock = ref(Date.now())
const notificationConfig = reactive({ default_channel_ids: [], repeat_interval_minutes: 60, recovery_notification: true, routes: [], updated_at: '' })
const enabledNotificationChannels = computed(() => notificationChannels.value.filter(channel => channel.status === 1))
const alertResult = computed(() => parseResult(currentAlert.value?.last_result))
const alertLabelSections = computed(() => [
  { key: 'system_labels', title: '系统标签', items: labelItems({ alertname: alertResult.value.alertname || currentAlert.value?.title, severity: currentAlert.value?.severity || alertResult.value.severity, datasource: alertResult.value.datasource }) },
  { key: 'datasource_labels', title: '数据源标签', items: labelItems(alertResult.value.datasource_labels) },
  { key: 'external_labels', title: '额外标签', items: labelItems(alertResult.value.external_labels) },
  { key: 'labels', title: 'PromQL 标签', items: labelItems(alertResult.value.labels) },
])
const groupingRows = computed(() => groupingPolicies.value.map(row => ({ ...row, enabled: row.status === 1, wait_time: formatDuration(row.wait_seconds), interval_time: formatDuration(row.interval_seconds) })))
const filteredGroupingRows = computed(() => groupingRows.value.filter(row => { const text = `${row.name} ${matcherSummary(row.matchers)} ${selectorSummary(row.group_by)}`.toLowerCase(); return text.includes(groupingKeyword.value.trim().toLowerCase()) }))
const filteredRules = computed(() => rules.value.filter(row => { const text = `${row.name} ${matcherSummary(row.source_matchers)} ${matcherSummary(row.target_matchers)} ${selectorSummary(row.equal_labels)}`.toLowerCase(); return text.includes(inhibitionKeyword.value.trim().toLowerCase()) }))
const filteredSilenceRules = computed(() => silencePolicies.value.filter(row => row.name.toLowerCase().includes(silenceKeyword.value.trim().toLowerCase())))
const pagedGroupingRows = computed(() => paginate(filteredGroupingRows.value, groupingPage.value, groupingPageSize.value))
const pagedRules = computed(() => paginate(filteredRules.value, inhibitionPage.value, inhibitionPageSize.value))
const pagedSilenceRules = computed(() => paginate(filteredSilenceRules.value, silencePage.value, silencePageSize.value))
watch(groupingKeyword, () => { groupingPage.value = 1 })
watch(inhibitionKeyword, () => { inhibitionPage.value = 1 })
watch(silenceKeyword, () => { silencePage.value = 1 })
watch(() => filteredGroupingRows.value.length, totalRows => { groupingPage.value = validPage(groupingPage.value, totalRows, groupingPageSize.value) })
watch(() => filteredRules.value.length, totalRows => { inhibitionPage.value = validPage(inhibitionPage.value, totalRows, inhibitionPageSize.value) })
watch(() => filteredSilenceRules.value.length, totalRows => { silencePage.value = validPage(silencePage.value, totalRows, silencePageSize.value) })
const ruleRules = { name: [{ required: true, message: '请输入抑制规则名称', trigger: 'blur' }] }
const silenceFormRules = {
  name: [{ required: true, message: '请输入策略名称', trigger: 'blur' }],
  date_range: [{ type: 'array', required: true, min: 2, message: '请选择静默时段', trigger: 'change' }],
  weekdays: [{ type: 'array', required: true, min: 1, message: '请至少选择一个生效星期', trigger: 'change' }],
  daily_time_range: [{ type: 'array', required: true, min: 2, message: '请选择每日静默时间', trigger: 'change' }],
}
let silenceClockTimer
onMounted(() => {
  loadPage()
  silenceClockTimer = window.setInterval(() => {
    if (activeTab.value === 'silence') silenceClock.value = Date.now()
  }, 1000)
})
onBeforeUnmount(() => window.clearInterval(silenceClockTimer))

async function loadPage() {
  pageLoading.value = true
  try {
    const [centerRes, groupingRes, ruleRes, silenceRes, notificationRes, channelRes] = await Promise.all([getFaultCenters(), getGroupingRules(route.params.id), getInhibitionRules(route.params.id), getSilenceRules(route.params.id), getFaultCenterNotificationConfig(route.params.id), getNotificationChannels()])
    center.value = (centerRes.data || []).find(item => item.id === Number(route.params.id)) || null
    groupingPolicies.value = groupingRes.data || []
    rules.value = ruleRes.data || []
    silencePolicies.value = silenceRes.data || []
    notificationChannels.value = channelRes.data || []
    syncNotificationConfig(notificationRes.data || {})
    if (center.value) await loadAlerts()
  } catch { center.value = null } finally { pageLoading.value = false }
}

function handleTabChange(name) { page.value = 1; keyword.value = ''; Object.assign(alertFilters, { severity: '', alert_status: '', start_date: '', end_date: '' }); if (['active', 'history'].includes(name)) loadAlerts(); if (name === 'grouping') loadGroupingRules(); if (name === 'silence') loadSilenceRules(); if (name === 'notification') reloadNotificationConfig() }
async function loadAlerts() { alertLoading.value = true; try { const defaultStatus = activeTab.value === 'history' ? 'history' : activeTab.value === 'silence' ? 'silenced' : 'active'; const res = await getAlerts({ fault_center_id: center.value.id, status: activeTab.value === 'active' && alertFilters.alert_status ? alertFilters.alert_status : defaultStatus, keyword: keyword.value, severity: alertFilters.severity || undefined, start_date: alertFilters.start_date || undefined, end_date: alertFilters.end_date || undefined, page: page.value, page_size: pageSize.value }); alerts.value = res.data?.list || []; total.value = Number(res.data?.total || 0) } finally { alertLoading.value = false } }
function searchAlerts(value) { if (value && typeof value === 'object') { keyword.value = String(value.keyword || '').trim(); Object.assign(alertFilters, { severity: value.severity || '', alert_status: value.alert_status || '', start_date: value.start_date || '', end_date: value.end_date || '' }) } else keyword.value = String(value || '').trim(); page.value = 1; loadAlerts() }
function changePage(value) { page.value = value; loadAlerts() }
function changePageSize(value) { pageSize.value = value; page.value = 1; loadAlerts() }
function showAlertDetail(row) { currentAlert.value = row; alertEvents.value = []; eventLoaded.value = false; eventLoading.value = false; lifecycleExpanded.value = false; detailVisible.value = true }
async function toggleLifecycle() {
  lifecycleExpanded.value = !lifecycleExpanded.value
  if (!lifecycleExpanded.value || eventLoaded.value || eventLoading.value || !currentAlert.value) return
  const alertId = currentAlert.value.id
  eventLoading.value = true
  try {
    const response = await getAlertEvents(alertId)
    if (currentAlert.value?.id === alertId) {
      alertEvents.value = response.data || []
      eventLoaded.value = true
    }
  } finally {
    if (currentAlert.value?.id === alertId) eventLoading.value = false
  }
}
function eventTitle(item) { if (item.event_type === 'created') return `创建${item.to_status === 'firing' ? '告警' : '预告警'}（${item.to_severity || '—'}）`; if (item.event_type === 'snapshot') return `历史状态快照：${displayStatus({ status: item.to_status })}（${item.to_severity || '—'}）`; if (item.event_type === 'severity_changed') return `告警等级 ${item.from_severity || '—'} → ${item.to_severity || '—'}`; if (item.event_type === 'state_changed') return `状态 ${displayStatus({ status: item.from_status })} → ${displayStatus({ status: item.to_status })}`; if (item.event_type === 'evaluation_status_changed') return `评估状态变为 ${item.evaluation_status}`; if (item.event_type === 'notification_sent') return item.reason === 'resolved' ? '恢复通知已发送' : '告警通知已发送'; if (item.event_type === 'notification_skipped') return '告警未发送通知'; if (item.event_type === 'notification_failed') return item.reason === 'resolved' ? '恢复通知发送失败' : '告警通知发送失败'; if (item.event_type === 'silenced') return '告警已静默'; if (item.event_type === 'unsilenced') return '告警已解除静默'; return item.event_type }
function eventTimelineType(item) { if (item.event_type === 'notification_failed') return 'danger'; if (item.event_type === 'notification_sent' || item.to_status === 'resolved') return 'success'; if (item.event_type === 'silenced' || item.event_type === 'unsilenced') return 'info'; if (item.event_type === 'created') return 'warning'; return 'primary' }
function eventReason(reason) { return ({ condition_detected: '首次命中告警条件', for_duration_reached: '持续命中达到配置时间', condition_retriggered: '恢复等待期间再次命中', severity_condition_changed: '命中的最高告警等级发生变化', condition_cleared: '告警条件不再命中', condition_cleared_before_firing: '持续时间内条件已恢复', recovery_wait_reached: '持续未命中达到恢复等待时间', resolved_before_first_notification: '在首次分组通知前恢复，告警事件已保留', retriggered_before_recovery_notification: '恢复通知发送前，同一告警已重新触发', rule_disabled: '规则已停用', rule_deleted: '规则已删除', datasource_unbound: '数据源已解除绑定', no_data: 'PromQL 查询无数据', error: 'PromQL 查询失败', migration_snapshot: '事件时间线上线前的告警，仅保留当时状态快照', probe_failed: '拨测失败', probe_recovered: '拨测恢复', certificate_domain_unhealthy: '证书域名异常', certificate_domain_changed: '证书域名状态发生变化', certificate_domain_recovered: '证书域名恢复正常', manual_unsilence: '人工解除静默' })[reason] || reason }
async function loadGroupingRules() { groupingLoading.value = true; try { groupingPolicies.value = (await getGroupingRules(center.value.id)).data || [] } finally { groupingLoading.value = false } }
function searchGroupings() { groupingPage.value = 1 }
async function reloadRules() { rules.value = (await getInhibitionRules(center.value.id)).data || [] }
async function searchInhibitions() { inhibitionPage.value = 1; await reloadRules() }
async function loadSilenceRules() { silenceLoading.value = true; try { silencePolicies.value = (await getSilenceRules(center.value.id)).data || [] } finally { silenceLoading.value = false } }
function searchSilences() { loadSilenceRules() }

function syncNotificationConfig(data) {
  Object.assign(notificationConfig, {
    default_channel_ids: [...(data.default_channel_ids || [])],
    repeat_interval_minutes: Math.max(1, Math.round(Number(data.repeat_interval_seconds || 3600) / 60)),
    recovery_notification: data.recovery_notification !== false,
    routes: (data.routes || []).map(item => ({ id: item.id, label_namespace: item.label_namespace || 'labels', label_key: item.label_key || '', label_value: item.label_value || '', channel_ids: [...(item.channel_ids || [])], status: item.status === 0 ? 0 : 1 })),
    updated_at: data.updated_at || '',
  })
  notificationSnapshot.value = JSON.parse(JSON.stringify(notificationConfig))
}
async function reloadNotificationConfig() { const [configRes, channelsRes] = await Promise.all([getFaultCenterNotificationConfig(center.value.id), getNotificationChannels()]); notificationChannels.value = channelsRes.data || []; syncNotificationConfig(configRes.data || {}) }
function cancelNotificationEdit() { if (notificationSnapshot.value) Object.assign(notificationConfig, JSON.parse(JSON.stringify(notificationSnapshot.value))); notificationEditing.value = false }
function addNotificationRoute() { notificationConfig.routes.push({ label_namespace: 'labels', label_key: '', label_value: '', channel_ids: [], status: 1 }) }
function removeNotificationRoute(index) { notificationConfig.routes.splice(index, 1) }
async function saveNotificationConfig() {
  if (notificationConfig.routes.some(item => !item.label_namespace || !item.label_key.trim() || !item.label_value.trim() || !item.channel_ids.length)) return ElMessage.warning('请完整填写告警路由')
  savingNotification.value = true
  try {
    await updateFaultCenterNotificationConfig(center.value.id, {
      default_channel_ids: notificationConfig.default_channel_ids,
      repeat_interval_seconds: Number(notificationConfig.repeat_interval_minutes) * 60,
      recovery_notification: notificationConfig.recovery_notification,
      routes: notificationConfig.routes.map(item => ({ label_namespace: item.label_namespace, label_key: item.label_key.trim(), label_value: item.label_value.trim(), channel_ids: item.channel_ids, status: item.status })),
    })
    await reloadNotificationConfig()
    notificationEditing.value = false
    ElMessage.success('通知配置已保存')
  } finally { savingNotification.value = false }
}

function startNameEdit() {
  nameDraft.value = center.value?.name || ''
  nameEditing.value = true
  nextTick(() => nameInputRef.value?.focus())
}

function cancelNameEdit() {
  if (nameSaving.value) return
  nameDraft.value = center.value?.name || ''
  nameEditing.value = false
}

async function saveCenterName() {
  if (nameSaving.value) return
  const nextName = nameDraft.value.trim()
  if (!nextName) return ElMessage.warning('故障中心名称不能为空')
  if (nextName === center.value.name) return cancelNameEdit()
  nameSaving.value = true
  try {
    await updateFaultCenter(center.value.id, {
      name: nextName,
      description: center.value.description || '',
      status: center.value.status,
    })
    center.value = { ...center.value, name: nextName }
    nameEditing.value = false
    ElMessage.success('名称已更新')
  } finally {
    nameSaving.value = false
  }
}

function openGroupingDrawer(row) {
  Object.assign(grouping, row ? {
    id: row.id,
    name: row.name,
    matchers: cloneMatchers(row.matchers),
    group_by: cloneSelectors(row.group_by),
    wait_seconds: Number(row.wait_seconds),
    interval_seconds: Number(row.interval_seconds),
    priority: Number(row.priority),
    enabled: row.status === 1,
  } : { id: null, name: '', matchers: [], group_by: [], wait_seconds: 30, interval_seconds: 300, priority: 100, enabled: true })
  groupingDrawerVisible.value = true
}

async function removeGroupingPolicy(row) {
  await ElMessageBox.confirm(`确认删除分组策略“${row.name}”？待发送分组会被释放，告警将在下一次评估时重新匹配策略。`, '删除分组策略', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  await deleteGroupingRule(center.value.id, row.id)
  await loadGroupingRules()
  ElMessage.success('分组策略已删除')
}

async function saveGrouping() {
  grouping.name = grouping.name.trim()
  if (!grouping.name) return ElMessage.warning('请输入策略名称')
  if (grouping.matchers.length > 10) return ElMessage.warning('匹配条件最多配置 10 组')
  if (grouping.group_by.length > 10) return ElMessage.warning('分组标签最多配置 10 个')
  if (!validOptionalMatchers(grouping.matchers)) return ElMessage.warning('请完整填写匹配标签来源、Key 和 Value，且不要重复')
  if (!validSelectors(grouping.group_by)) return ElMessage.warning('请完整填写分组标签来源和 Key，且不要重复')
  savingGrouping.value = true
  try {
    const payload = {
      name: grouping.name,
      matchers: cloneMatchers(grouping.matchers).map(item => ({ ...item, key: item.key.trim(), value: item.value.trim() })),
      group_by: cloneSelectors(grouping.group_by).map(item => ({ ...item, key: item.key.trim() })),
      wait_seconds: Number(grouping.wait_seconds),
      interval_seconds: Number(grouping.interval_seconds),
      priority: Number(grouping.priority),
      status: grouping.enabled ? 1 : 0,
    }
    if (grouping.id) await updateGroupingRule(center.value.id, grouping.id, payload)
    else await createGroupingRule(center.value.id, payload)
    await loadGroupingRules()
    groupingDrawerVisible.value = false
    ElMessage.success(grouping.id ? '分组策略已更新' : '分组策略已创建')
  } finally { savingGrouping.value = false }
}
function cloneMatchers(values) { return (values || []).map(item => ({ namespace: item.namespace || 'labels', key: item.key || '', value: item.value || '' })) }
function cloneSelectors(values) { return (values || []).map(item => ({ namespace: item.namespace || 'labels', key: item.key || '' })) }
function validOptionalMatchers(values) {
  const identities = new Set()
  return values.every(item => {
    const identity = `${item.namespace}\u0000${item.key.trim()}`
    if (!item.namespace || !item.key.trim() || !item.value.trim() || identities.has(identity)) return false
    identities.add(identity)
    return true
  })
}
function validSelectors(values) {
  const identities = new Set()
  return values.every(item => {
    const identity = `${item.namespace}\u0000${item.key.trim()}`
    if (!item.namespace || !item.key.trim() || identities.has(identity)) return false
    identities.add(identity)
    return true
  })
}
function addGroupingSelector() { if (grouping.group_by.length < 10) grouping.group_by.push(emptySelector()); else ElMessage.warning('分组标签最多配置 10 个') }
function removeGroupingSelector(index) { grouping.group_by.splice(index, 1) }
function addGroupingMatcher() { if (grouping.matchers.length < 10) grouping.matchers.push(emptyMatcher()); else ElMessage.warning('匹配条件最多配置 10 组') }
function removeGroupingMatcher(index) { grouping.matchers.splice(index, 1) }
function openRuleDrawer(row) { Object.assign(ruleForm, row ? { id: row.id, name: row.name, source_matchers: cloneMatchers(row.source_matchers), target_matchers: cloneMatchers(row.target_matchers), equal_labels: cloneSelectors(row.equal_labels), enabled: row.status === 1 } : { id: null, name: '', source_matchers: [emptyMatcher()], target_matchers: [emptyMatcher()], equal_labels: [], enabled: true }); if (!ruleForm.source_matchers.length) ruleForm.source_matchers = [emptyMatcher()]; if (!ruleForm.target_matchers.length) ruleForm.target_matchers = [emptyMatcher()]; ruleDrawerVisible.value = true }
function addRuleMatcher(field) { if (ruleForm[field].length < 10) ruleForm[field].push(emptyMatcher()); else ElMessage.warning('匹配标签最多配置 10 组') }
function removeRuleMatcher(field, index) { ruleForm[field].splice(index, 1); if (!ruleForm[field].length) ruleForm[field].push(emptyMatcher()) }
function addEqualLabel() { if (ruleForm.equal_labels.length < 10) ruleForm.equal_labels.push(emptySelector()); else ElMessage.warning('相等标签最多配置 10 组') }
function removeEqualLabel(index) { ruleForm.equal_labels.splice(index, 1) }
function validMatchers(values) { return values.length > 0 && values.every(item => item.namespace && item.key.trim() && item.value.trim()) }
async function saveRule() { const valid = await ruleFormRef.value?.validate().catch(() => false); if (!valid) return; if (!validMatchers(ruleForm.source_matchers) || !validMatchers(ruleForm.target_matchers) || ruleForm.equal_labels.some(item => !item.namespace || !item.key.trim())) return ElMessage.warning('请完整填写标签来源、Key 和 Value'); savingRule.value = true; try { const payload = { name: ruleForm.name.trim(), source_matchers: cloneMatchers(ruleForm.source_matchers).map(item => ({ ...item, key: item.key.trim(), value: item.value.trim() })), target_matchers: cloneMatchers(ruleForm.target_matchers).map(item => ({ ...item, key: item.key.trim(), value: item.value.trim() })), equal_labels: cloneSelectors(ruleForm.equal_labels).map(item => ({ ...item, key: item.key.trim() })), status: ruleForm.enabled ? 1 : 0 }; if (ruleForm.id) await updateInhibitionRule(center.value.id, ruleForm.id, payload); else await createInhibitionRule(center.value.id, payload); rules.value = (await getInhibitionRules(center.value.id)).data || []; ruleDrawerVisible.value = false; ElMessage.success(ruleForm.id ? '抑制规则已更新' : '抑制规则已创建') } finally { savingRule.value = false } }
async function removeRule(row) { await ElMessageBox.confirm(`确认删除抑制规则“${row.name}”？`, '删除规则', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }); await deleteInhibitionRule(center.value.id, row.id); rules.value = (await getInhibitionRules(center.value.id)).data || []; ElMessage.success('抑制规则已删除') }

function openSilenceDrawer(row) {
  const now = new Date(), later = new Date(now.getTime() + 60 * 60 * 1000)
  if (row?.silence_type) {
    Object.assign(silenceForm, {
      id: row.id,
      name: row.name,
      matchers: cloneMatchers(row.matchers),
      silence_type: row.silence_type,
      date_range: row.silence_type === 'once' ? [row.start_at, row.end_at] : [],
      weekdays: [...(row.weekdays || [])].sort((a, b) => a - b),
      daily_time_range: row.start_time && row.end_time ? [row.start_time, row.end_time] : [],
      enabled: row.status === 1,
    })
  } else {
    Object.assign(silenceForm, {
      id: null,
      name: row?.title ? `${row.title} 静默` : '',
      matchers: [firstAlertMatcher(row)],
      silence_type: 'once',
      date_range: [now.toISOString(), later.toISOString()],
      weekdays: [1, 2, 3, 4, 5],
      daily_time_range: ['09:00', '18:00'],
      enabled: true,
    })
  }
  if (!silenceForm.matchers.length) silenceForm.matchers = [emptyMatcher()]
  silenceDrawerVisible.value = true
}
function firstAlertMatcher(row) { const result = parseResult(row?.last_result); const alertname = result.alertname || row?.title; if (alertname) return { namespace: 'system_labels', key: 'alertname', value: String(alertname) }; for (const namespace of ['labels', 'datasource_labels', 'external_labels']) { const entry = Object.entries(result[namespace] || {})[0]; if (entry) return { namespace, key: entry[0], value: String(entry[1]) } } return emptyMatcher() }
function addSilenceMatcher() { if (silenceForm.matchers.length < 10) silenceForm.matchers.push(emptyMatcher()); else ElMessage.warning('匹配标签最多配置 10 组') }
function removeSilenceMatcher(index) { silenceForm.matchers.splice(index, 1); if (!silenceForm.matchers.length) addSilenceMatcher() }
function sortSilenceWeekdays() { silenceForm.weekdays.sort((a, b) => a - b) }
async function saveSilenceRule() {
  const valid = await silenceFormRef.value?.validate().catch(() => false)
  if (!valid) return
  const matchers = []
  for (const item of silenceForm.matchers) {
    const key = item.key.trim(), value = item.value.trim()
    if (!item.namespace || !key || !value) return ElMessage.warning('请完整填写标签来源、Key 和 Value')
    matchers.push({ namespace: item.namespace, key, value })
  }
  const weekly = silenceForm.silence_type === 'weekly'
  const payload = {
    name: silenceForm.name.trim(),
    matchers,
    silence_type: silenceForm.silence_type,
    weekdays: weekly ? [...silenceForm.weekdays] : [],
    start_time: weekly ? silenceForm.daily_time_range[0] : '',
    end_time: weekly ? silenceForm.daily_time_range[1] : '',
    start_at: weekly ? null : silenceForm.date_range[0],
    end_at: weekly ? null : silenceForm.date_range[1],
    status: silenceForm.enabled ? 1 : 0,
  }
  savingSilence.value = true
  try {
    if (silenceForm.id) await updateSilenceRule(center.value.id, silenceForm.id, payload)
    else await createSilenceRule(center.value.id, payload)
    silenceDrawerVisible.value = false
    ElMessage.success(silenceForm.id ? '静默策略已更新' : '静默策略已创建')
    await Promise.all([loadSilenceRules(), refreshData()])
  } finally { savingSilence.value = false }
}
async function removeSilenceRule(row) { await ElMessageBox.confirm(`确认删除静默策略“${row.name}”？`, '删除策略', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }); await deleteSilenceRule(center.value.id, row.id); ElMessage.success('静默策略已删除'); await Promise.all([loadSilenceRules(), refreshData()]) }
async function refreshData() { const centerRes = await getFaultCenters(); center.value = (centerRes.data || []).find(item => item.id === Number(route.params.id)) || center.value; await loadAlerts() }

function silenceRuleActiveAt(row, nowMs = silenceClock.value) {
  if (row?.silence_type !== 'weekly') {
    const startMs = new Date(row?.start_at).getTime(), endMs = new Date(row?.end_at).getTime()
    return Number.isFinite(startMs) && Number.isFinite(endMs) && nowMs >= startMs && nowMs < endMs
  }
  const now = new Date(nowMs)
  const nowSecond = now.getHours() * 3600 + now.getMinutes() * 60 + now.getSeconds()
  const [startHour, startMinute] = String(row.start_time || '').split(':').map(Number)
  const [endHour, endMinute] = String(row.end_time || '').split(':').map(Number)
  if (![startHour, startMinute, endHour, endMinute].every(Number.isFinite)) return false
  const startSecond = startHour * 3600 + startMinute * 60
  const endSecond = endHour * 3600 + endMinute * 60
  let weekday = now.getDay() || 7
  if (endSecond < startSecond && nowSecond < endSecond) weekday = weekday === 1 ? 7 : weekday - 1
  if (!(row.weekdays || []).includes(weekday)) return false
  return endSecond > startSecond
    ? nowSecond >= startSecond && nowSecond < endSecond
    : nowSecond >= startSecond || nowSecond < endSecond
}
function silenceState(row) {
  const nowMs = silenceClock.value
  if (row?.status !== 1) return { text: '已停用', type: 'info' }
  if (row?.silence_type === 'weekly') return silenceRuleActiveAt(row, nowMs) ? { text: '静默中', type: 'success' } : { text: '等待时段', type: 'primary' }
  const startMs = new Date(row?.start_at).getTime(), endMs = new Date(row?.end_at).getTime()
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) return { text: '时间异常', type: 'danger' }
  if (nowMs < startMs) return { text: '待生效', type: 'warning' }
  if (nowMs >= endMs) return { text: '已过期', type: 'info' }
  if (silenceRuleActiveAt(row, nowMs)) return { text: '静默中', type: 'success' }
  return { text: '等待时段', type: 'primary' }
}
function silenceScheduleText(row) {
  if (row?.silence_type !== 'weekly') return `${formatTime(row?.start_at)} 至 ${formatTime(row?.end_at)}`
  const names = (row.weekdays || []).map(value => weekdayOptions.find(item => item.value === value)?.label).filter(Boolean).join('、')
  return `${names || '未选择星期'} ${row.start_time || '—'}–${row.end_time || '—'}`
}
function statusType(value) { return ({ pending: 'warning', firing: 'danger', recovering: 'primary', resolved: 'success', terminated: 'info' })[value] || 'info' }
function severityType(value) { return ({ P0: 'danger', P1: 'warning', P2: 'primary', critical: 'danger', warning: 'warning' })[value] || 'info' }
function isSilenced(row) { return !!row?.silenced_until && new Date(row.silenced_until).getTime() > Date.now() }
function displayStatus(row) { return ({ pending: '预告警', firing: '告警中', recovering: '恢复中', resolved: '已恢复', terminated: '已终止' })[row?.status] || row?.status || '—' }
function effectiveNotificationStatus(row) { return isSilenced(row) ? 'silenced' : row?.notification_status }
function notificationStatusText(row) { return ({ waiting_group: '分组中', waiting_update: '分组中', waiting_recovery: '分组中', sent: '已发送', recovery_sent: '恢复已发送', silenced: '静默中', inhibited: '抑制中', no_receiver: '无通知对象', send_failed: '发送失败', resolved_unnotified: '未通知已恢复', recovery_disabled: '恢复通知关闭', terminated: '已终止' })[effectiveNotificationStatus(row)] || (row?.status === 'pending' ? '尚未触发' : '待处理') }
function notificationStatusType(status) { return ({ waiting_group: 'warning', waiting_update: 'warning', waiting_recovery: 'primary', sent: 'success', recovery_sent: 'success', silenced: 'info', inhibited: 'info', no_receiver: 'info', send_failed: 'danger', resolved_unnotified: 'info', recovery_disabled: 'info', terminated: 'info' })[status] || 'info' }
function isPrometheusAlert(row) { return row?.source === 'prometheus-rule' }
function canOpenAlertRule(row) { return isPrometheusAlert(row) && Number(row?.rule_group_id) > 0 && Number(row?.source_id) > 0 }
function alertRuleEditRoute(row) { return { name: 'AlertRuleEdit', params: { groupId: row.rule_group_id, ruleId: row.source_id } } }
function hasUpcomingNotification(row) { return Boolean(row?.next_notification_at) && ['waiting_group', 'waiting_update', 'waiting_recovery'].includes(effectiveNotificationStatus(row)) }
function showNotificationReason(row) { return Boolean(row?.notification_reason) && ['silenced', 'inhibited', 'no_receiver', 'send_failed', 'resolved_unnotified', 'recovery_disabled', 'terminated'].includes(effectiveNotificationStatus(row)) }
function alertSourceText(row) { return row?.source === 'prometheus-rule' ? '指标规则' : row?.source === 'probe-task' ? '拨测任务' : row?.source === 'scheduled-task' ? '定时任务' : row?.source === 'certificate-domain' ? '证书域名' : row?.source || '未知来源' }
function formatTime(value) { if (!value) return '—'; const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false }) }
function alertTriggerTime(row) { return row?.fired_at || row?.started_at || row?.state_changed_at || '' }
function alertDurationSeconds(row, currentTimestamp = Date.now()) { const triggerTime = alertTriggerTime(row); if (!triggerTime) return null; const start = new Date(triggerTime).getTime(); const resolved = row.resolved_at ? new Date(row.resolved_at).getTime() : currentTimestamp; if (!Number.isFinite(start) || !Number.isFinite(resolved) || resolved < start) return null; return Math.floor((resolved - start) / 1000) }
function formatAlertDuration(row, currentTimestamp = Date.now()) { const seconds = alertDurationSeconds(row, currentTimestamp); if (seconds === null) return '—'; const days = Math.floor(seconds / 86400); const hours = Math.floor((seconds % 86400) / 3600); const minutes = Math.floor((seconds % 3600) / 60); const remainSeconds = seconds % 60; if (days > 0) return `${days}天 ${hours}小时 ${minutes}分 ${remainSeconds}秒`; if (hours > 0) return `${hours}小时 ${minutes}分 ${remainSeconds}秒`; if (minutes > 0) return `${minutes}分 ${remainSeconds}秒`; return `${seconds}秒` }
function durationMeterSegments(seconds) { if (seconds <= 0) return 1; const minimum = 10, maximum = 86400; const normalized = (Math.log(Math.max(seconds, minimum)) - Math.log(minimum)) / (Math.log(maximum) - Math.log(minimum)); return Math.max(1, Math.ceil(Math.min(1, Math.max(0, normalized)) * 10)) }
function formatDuration(seconds) { const value = Number(seconds || 0); if (value >= 3600 && value % 3600 === 0) return `${value / 3600} 小时`; if (value >= 60 && value % 60 === 0) return `${value / 60} 分钟`; return value > 0 ? `${value} 秒` : '—' }
function formatResult(value) { if (!value) return '—'; try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value } }
function parseResult(value) { if (!value) return {}; if (typeof value === 'object') return value; try { const parsed = JSON.parse(value); return parsed && typeof parsed === 'object' ? parsed : {} } catch { return {} } }
function labelItems(value) { if (!value || typeof value !== 'object' || Array.isArray(value)) return []; return Object.entries(value).filter(([, itemValue]) => itemValue !== undefined && itemValue !== null && String(itemValue).trim() !== '').sort(([left], [right]) => left.localeCompare(right)).map(([key, itemValue]) => ({ key, value: String(itemValue) })) }
function labelToneClass(key) { const normalized = String(key || '').trim().toLowerCase(); const preferred = { env: 'yellow', owner: 'purple', service: 'blue' }; if (preferred[normalized]) return `scope-tag-${preferred[normalized]}`; const tones = ['blue', 'yellow', 'purple', 'cyan', 'green']; const hash = [...normalized].reduce((total, character) => total + character.charCodeAt(0), 0); return `scope-tag-${tones[hash % tones.length]}` }
function labelNamespaceText(value) { return labelNamespaceOptions.find(item => item.value === value)?.label || value || '未知来源' }
function matcherSummary(values) { return values?.length ? values.map(item => `${labelNamespaceText(item.namespace)}.${item.key}=${item.value}`).join(' 且 ') : '—' }
function selectorSummary(values) { return values?.length ? values.map(item => `${labelNamespaceText(item.namespace)}.${item.key}`).join('、') : '—' }
function validPage(currentPage, totalRows, currentPageSize) { return Math.min(currentPage, Math.max(1, Math.ceil(totalRows / currentPageSize))) }
function paginate(rows, currentPage, currentPageSize) { const start = (validPage(currentPage, rows.length, currentPageSize) - 1) * currentPageSize; return rows.slice(start, start + currentPageSize) }
</script>

<style scoped>
.detail-page :deep(.el-tag){border-radius:0}
.detail-page :deep(.severity-tag){min-width:36px;justify-content:center;font-weight:600}
.detail-monospace{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;font-weight:500!important}
.detail-page{--ink:var(--ops-text);--muted:var(--ops-muted);--border:var(--ops-border);display:flex;min-height:calc(100vh - 108px);flex-direction:column;gap:15px}.detail-hero{display:flex;align-items:center;gap:14px;padding:2px 0 4px}.hero-main{min-width:0;flex:1}.hero-title{display:flex;align-items:center;gap:12px}.hero-title h1{display:block!important;max-width:680px;margin:0;overflow:hidden;color:var(--ops-text);font-size:21px;font-weight:680;letter-spacing:0;text-overflow:ellipsis;white-space:nowrap}.hero-main p{margin:6px 0 0;overflow:hidden;color:var(--muted);font-size:12px;text-overflow:ellipsis;white-space:nowrap}.health-state{display:inline-flex;align-items:center;gap:6px;color:#188864;font-size:11px}.health-state i{width:7px;height:7px;background:#1aa174;border-radius:50%;box-shadow:0 0 0 4px rgba(26,161,116,.1)}.health-state.danger{color:#c74646}.health-state.danger i{background:#dc5555;box-shadow:0 0 0 4px rgba(220,85,85,.1)}.metric-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.metric-card{display:flex;min-height:108px;flex-direction:column;padding:17px 18px;background:var(--ops-surface);border:1px solid var(--border);border-top:3px solid #aeb9c8;border-radius:7px}.metric-card>span{color:var(--ops-muted);font-size:11px}.metric-card strong{margin-top:8px;color:var(--ops-text);font-size:25px;font-weight:680;line-height:1}.metric-card small{margin-top:auto;color:var(--ops-muted);font-size:10px}.metric-card.firing{border-top-color:#dc5b5b}.metric-card.firing strong{color:#c94d4d}.metric-card.pending{border-top-color:#e7a63c}.metric-card.silenced{border-top-color:#7c8796}.metric-card.resolved{border-top-color:#20a079}.policy-bar{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));overflow:hidden;background:var(--ops-surface);border:1px solid var(--border);border-radius:7px}.policy-bar>div{display:flex;min-width:0;flex-direction:column;gap:5px;padding:12px 18px;border-right:1px solid var(--ops-border-soft)}.policy-bar>div:last-child{border-right:0}.policy-bar span{color:var(--ops-muted);font-size:10px}.policy-bar strong{overflow:hidden;color:var(--ops-text);font-size:12px;font-weight:600;text-overflow:ellipsis;white-space:nowrap}.workbench{min-height:510px;background:var(--ops-surface);border:1px solid var(--border);border-radius:7px}.center-tabs :deep(.el-tabs__header){margin:0;padding:0 22px;border-bottom:1px solid var(--ops-border-soft)}.center-tabs :deep(.el-tabs__nav-wrap::after){display:none}.center-tabs :deep(.el-tabs__item){height:52px;padding:0 21px;color:var(--ops-muted);font-size:13px}.center-tabs :deep(.el-tabs__item.is-active){color:var(--ops-primary);font-weight:600}.center-tabs :deep(.el-tabs__active-bar){height:2px;background:var(--ops-primary)}.center-tabs :deep(.el-tabs__content){padding:20px 22px 24px}.center-tabs :deep(.alert-section),.rule-section{min-height:400px}.center-tabs :deep(.alert-filter){display:flex;justify-content:flex-start;gap:8px}.center-tabs :deep(.alert-filter .el-input){width:min(340px,100%)}.center-tabs :deep(.alert-filter .filter-refresh){margin-left:auto}.center-tabs :deep(.alert-section .el-table__row){cursor:pointer}.center-tabs :deep(.alert-time-column .cell){white-space:nowrap}.center-tabs :deep(.duration-cell){display:flex;min-width:0;flex-direction:column;gap:6px}.center-tabs :deep(.duration-text){overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.center-tabs :deep(.duration-meter){width:100%;height:4px;overflow:hidden;background:#edf1f5;border-radius:999px}.center-tabs :deep(.duration-meter-fill){display:block;height:100%;border-radius:inherit;transition:width .4s linear,background-color .2s}.center-tabs :deep(.duration-meter-fill.is-short){background:var(--ops-primary)}.center-tabs :deep(.duration-meter-fill.is-medium){background:#e6a23c}.center-tabs :deep(.duration-meter-fill.is-long){background:#e67e22}.center-tabs :deep(.duration-meter-fill.is-critical){background:var(--ops-danger)}.center-tabs :deep(.table-wrap){position:relative;min-height:180px}.center-tabs :deep(.table-loading){position:absolute;z-index:4;inset:0;display:grid;place-items:center;color:var(--ops-primary);background:rgba(255,255,255,.72)}.center-tabs :deep(.pagination){display:flex;justify-content:flex-end;margin-top:16px}.strategy-layout{display:grid;min-height:400px;grid-template-columns:minmax(220px,.65fr) minmax(360px,1.35fr);gap:56px;padding:18px 10px}.strategy-copy{padding-top:2px}.strategy-copy strong{color:#293b53;font-size:15px;font-weight:600}.strategy-copy p{max-width:360px;margin:8px 0 0;color:#8491a3;font-size:11px;line-height:1.7}.strategy-form{max-width:560px}.strategy-form :deep(.el-select){width:100%}.section-heading{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px}.section-heading>div{display:flex;flex-direction:column;gap:4px}.section-heading strong{color:#293b53;font-size:14px}.section-heading span{color:var(--ops-muted);font-size:10px}.rule-section code{color:#4c6078;font:11px ui-monospace,SFMono-Regular,Menlo,monospace}.drawer-header{display:flex;width:100%;align-items:center;gap:12px}.back-button{height:32px;padding:0 8px;color:var(--ops-primary);font-size:14px}.drawer-title{color:var(--ops-text);font-size:16px;font-weight:600}.drawer-footer{display:flex;justify-content:flex-end;gap:8px}:global(.center-drawer.el-drawer){border-radius:0!important}:global(.center-drawer .el-drawer__header){margin-bottom:0;padding:18px 20px;border-bottom:1px solid #e5e9f2}:global(.center-drawer .el-drawer__body){padding:18px 20px;background:var(--ops-surface)}:global(.center-drawer .el-drawer__footer){padding:12px 20px;border-top:1px solid #e5e9f2}.alert-detail dl{display:grid;grid-template-columns:1fr 1fr;gap:1px;margin:0;padding:1px;background:var(--ops-border)}.alert-detail dl div{padding:12px 14px;background:var(--ops-surface)}.alert-detail dt{color:var(--ops-subtle);font-size:10px}.alert-detail dd{margin:5px 0 0;overflow-wrap:anywhere;color:#475467;font-size:12px;font-weight:600}.alert-detail section{margin-top:20px}.alert-detail section>span{color:#475467;font-size:12px;font-weight:600}.alert-detail pre{max-height:340px;margin:8px 0 0;padding:14px;overflow:auto;color:#d5deeb;background:#172235;border-radius:6px;font:11px/1.65 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap}@media(max-width:900px){.metric-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.strategy-layout{grid-template-columns:1fr;gap:24px}.center-tabs :deep(.el-tabs__header){overflow-x:auto}}@media(max-width:600px){.detail-hero{flex-wrap:wrap}.hero-main{width:calc(100% - 52px)}.metric-grid,.policy-bar{grid-template-columns:1fr}.policy-bar>div{border-right:0;border-bottom:1px solid var(--ops-border-soft)}.center-tabs :deep(.el-tabs__content){padding:16px}.center-tabs :deep(.alert-filter){flex-direction:column}.center-tabs :deep(.alert-filter .el-input){width:100%}.center-tabs :deep(.alert-filter .filter-refresh){align-self:flex-end;margin-left:0}.alert-detail dl{grid-template-columns:1fr}}
.detail-header{
  display:flex;
  min-height:32px;
  align-items:center;
  gap:14px;
}
.center-summary{min-width:0;flex:1}
.center-name{
  display:flex;
  min-width:0;
  align-items:center;
  gap:4px;
}
.center-name p{
  margin:0;
  overflow:hidden;
  color:var(--ops-muted,var(--ops-muted));
  font-size:12px;
  font-weight:400;
  line-height:1.5;
  text-overflow:ellipsis;
  white-space:nowrap;
}
.center-name-input{
  width:140px;
  max-width:min(140px,calc(100vw - 210px));
}
.center-name-input :deep(.el-input__wrapper){
  box-shadow:0 0 0 1px var(--ops-primary,var(--ops-primary)) inset;
}
.center-summary .el-button{
  width:28px;
  height:28px;
  flex:0 0 auto;
  color:var(--ops-muted);
}
.center-summary .el-button:hover{color:var(--ops-primary);background:var(--ops-primary-soft)}
.center-summary .name-action.is-confirm{color:#1a8f68}
.center-summary .name-action.is-confirm:hover{color:#137555;background:#ebf8f3}
.center-summary .name-action.is-cancel{color:#c55252}
.center-summary .name-action.is-cancel:hover{color:#a83f3f;background:#fff0f0}
/* 六个列表页签统一搜索区到表格的距离，包括子组件中的工具栏。 */
.center-tabs :deep(.alert-filter),
.center-tabs :deep(.strategy-toolbar) {
  margin: 0 0 16px;
  padding: 0;
}
.grouping-section{min-width:0}
.group-labels{display:flex;flex-wrap:wrap;gap:5px}
.all-in-one-group{color:var(--ops-primary);font-size:12px}
.grouping-section :deep(td.el-table__cell){padding-block:11px}
.center-tabs :deep(.alert-filter.is-advanced){justify-content:flex-start;flex-wrap:wrap}
.center-tabs :deep(.alert-filter.is-advanced .el-input){width:220px}
.center-tabs :deep(.alert-filter.is-advanced .alert-date-filter){width:220px;flex:0 0 220px}
.strategy-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px}
.strategy-toolbar>div{display:flex;align-items:center;flex-wrap:wrap;gap:8px}
.strategy-toolbar .el-input{width:240px}
.strategy-toolbar .el-select{width:130px}
.table-icon-actions{display:flex;align-items:center;justify-content:center;gap:8px}
.table-icon-actions .el-button+.el-button{margin-left:0}
@media(max-width:600px){
  .detail-header{padding-inline:14px}
  .center-tabs :deep(.alert-filter.is-advanced .el-input),
  .center-tabs :deep(.alert-filter.is-advanced .el-select),
  .center-tabs :deep(.alert-filter.is-advanced .el-date-editor){width:100%!important;flex-basis:auto}
  .strategy-toolbar{align-items:stretch;flex-direction:column}
  .strategy-toolbar>div,.strategy-toolbar .el-input,.strategy-toolbar .el-select{width:100%}
}
.grouping-form :deep(.el-form-item){margin-bottom:24px}
.grouping-timing-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}
.priority-input{width:100%}
.inhibition-form :deep(.el-form-item){margin-bottom:24px}
.form-label{display:inline-flex;align-items:center;gap:6px;color:var(--ops-text);font-weight:500}
.form-label .el-icon{color:var(--ops-muted);cursor:help}
:global(.config-drawer.el-drawer){max-width:100vw}
.number-field{display:flex;width:100%;align-items:center}
.number-field :deep(.el-input-number){min-width:0;flex:1}
.number-field>span{height:32px;flex:0 0 auto;padding:0 13px;color:var(--ops-muted);background:var(--ops-surface-soft);border:1px solid var(--ops-border);border-left:0;border-radius:0 4px 4px 0;font-size:12px;line-height:30px;white-space:nowrap}
.number-field :deep(.el-input-number .el-input__wrapper){border-radius:4px 0 0 4px}
@media(max-width:520px){.grouping-timing-grid{grid-template-columns:1fr;gap:0}}
.silence-form :deep(.el-date-editor){width:100%}
.silence-type-options{display:flex;width:100%}
.silence-type-options :deep(.el-radio-button){flex:1}
.silence-type-options :deep(.el-radio-button__inner){width:100%}
.label-match-list{display:flex;width:100%;flex-direction:column;gap:10px}
.label-match-item{position:relative;padding:12px;background:#f7f9fc;border-radius:4px}
.label-match-remove{position:absolute;z-index:1;top:4px;right:5px;width:28px;height:28px}
.label-source-control{display:flex;flex-direction:column;gap:6px}
.label-source-control>span,.label-field-control>span{color:#8a95a5;font-size:11px}
.label-source-options{display:flex;width:100%}
.label-source-options :deep(.el-radio-button){flex:1}
.label-source-options :deep(.el-radio-button__inner){width:100%;padding-inline:8px}
.label-match-fields{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:10px;margin-top:10px}
.label-field-control{display:flex;min-width:0;flex-direction:column;gap:6px}
.label-field-control .el-select,.label-field-control .el-input{width:100%}
.add-match-condition{width:100%;border-style:dashed}
@media(max-width:600px){.label-match-fields{grid-template-columns:1fr}}
.equal-label-key{margin-top:10px}
.grouping-label-key{margin-top:10px}
.notification-section{min-height:440px;container-name:notification;container-type:inline-size}
.notification-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:20px;margin-bottom:18px}
.notification-heading strong{display:flex;align-items:center;gap:8px;color:#263244;font-size:15px;line-height:1.5}
.notification-heading strong .el-icon{color:var(--ops-primary);font-size:17px}
.notification-actions{display:flex;flex:0 0 auto;gap:8px}
.notification-grid{display:grid;grid-template-columns:minmax(320px,.8fr) minmax(520px,1.2fr);align-items:start;gap:18px}
.notification-card{min-width:0;padding:20px;background:var(--ops-surface);border:1px solid var(--ops-border-soft);border-radius:8px;box-shadow:0 1px 3px rgba(30,45,65,.035)}
.notification-card h3{display:flex;align-items:center;gap:8px;margin:0 0 18px;padding-bottom:14px;color:var(--ops-text);border-bottom:1px solid var(--ops-border-soft);font-size:14px;font-weight:600}
.notification-card h3 .el-icon{color:var(--ops-primary);font-size:16px}
.notification-card h3 .notification-help-icon{color:var(--ops-muted);cursor:help;font-size:14px}
.notification-card h3 .notification-help-icon:hover,.notification-card h3 .notification-help-icon:focus-visible{color:var(--ops-primary);outline:none}
.notification-form :deep(.el-form-item){margin-bottom:22px}
.notification-form :deep(.el-form-item:last-child){margin-bottom:0}
.notification-form :deep(.el-select){width:100%}
.notification-form small{margin-left:10px;color:#98a2b1}
.route-table-head,.route-row{display:grid;grid-template-columns:minmax(0,.75fr) minmax(0,.7fr) minmax(0,.8fr) minmax(0,1.2fr) 42px;align-items:center;gap:8px}
.route-table-head{padding:0 4px 9px;color:var(--ops-muted);font-size:12px;font-weight:500}
.route-list{display:flex;flex-direction:column;gap:10px}
.route-row{padding:10px;background:#f8fafc;border:1px solid var(--ops-border-soft);border-radius:6px}
.route-row>*{min-width:0}
.route-row :deep(.el-input),.route-row :deep(.el-select){width:100%}
.route-row>span{color:#a0a8b4;text-align:center}
.route-row .el-button{width:32px;height:32px;justify-self:center}
.add-route{width:100%;margin-top:14px;border-style:dashed}
@container notification (max-width:1150px){.notification-grid{grid-template-columns:1fr}}
@media(max-width:1100px){.notification-grid{grid-template-columns:1fr}}
@media(max-width:700px){
  .notification-heading{align-items:stretch;flex-direction:column}
  .notification-actions .el-button{flex:1}
  .route-table-head{display:none}
  .route-row{grid-template-columns:1fr}
  .route-row .el-button{justify-self:end}
}
.alert-timeline :deep(.el-timeline){margin:12px 0 0;padding-left:8px}.alert-timeline :deep(.el-timeline-item__timestamp){color:var(--ops-subtle);font-size:10px}.alert-timeline strong{display:block;color:#475467;font-size:12px}.alert-timeline small{display:block;margin-top:3px;color:var(--ops-subtle);font-size:10px}
.alert-detail dd{font-weight:400}
.alert-detail dl .detail-item--full{grid-column:1/-1}
.notification-state-detail{display:flex;flex-direction:column;gap:3px}.notification-state-detail small{color:var(--ops-subtle);font-size:10px;line-height:1.5}
.detail-link{color:var(--ops-primary);text-decoration:none}.detail-link:hover{text-decoration:underline}.detail-link:focus-visible{outline:2px solid var(--ops-primary);outline-offset:2px}
.alert-label-sections{border:1px solid var(--ops-border)}.alert-label-section{display:grid;grid-template-columns:118px minmax(0,1fr);min-height:48px;border-bottom:1px solid var(--ops-border-soft)}.alert-label-section:last-child{border-bottom:0}.alert-label-section>span{padding:14px;color:var(--ops-muted);background:var(--ops-surface-soft);font-size:11px}.scope-tags{display:flex;align-items:center;align-content:center;flex-wrap:wrap;gap:6px;padding:9px 12px}.scope-tags small{color:#a0a8b4;font-size:11px}.scope-tag{display:inline-flex;min-width:0;height:24px;align-items:center;padding:0 8px;overflow:hidden;border:1px solid transparent;border-radius:4px;font-size:12px;line-height:22px;text-overflow:ellipsis;white-space:nowrap}.scope-tag-yellow{color:#ad6800;background:#fffbe6;border-color:#ffe58f}.scope-tag-purple{color:#531dab;background:#f9f0ff;border-color:#d3adf7}.scope-tag-blue{color:#1d39c4;background:#f0f5ff;border-color:#adc6ff}.scope-tag-cyan{color:#006d75;background:#e6fffb;border-color:#87e8de}.scope-tag-green{color:#237804;background:#f6ffed;border-color:#b7eb8f}
.detail-section-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;color:#475467}
.detail-section-heading>span{font-size:13px;font-weight:500}
.detail-section-heading>small{color:var(--ops-subtle);font-size:11px;font-weight:400}
.alert-timeline{padding:0;border:1px solid var(--ops-border-soft);background:var(--ops-surface-soft)}
.timeline-toggle{display:flex;width:100%;align-items:center;justify-content:space-between;padding:14px 16px;color:#475467;text-align:left;cursor:pointer;background:transparent;border:0;font:inherit}
.timeline-toggle:focus-visible{outline:2px solid var(--ops-primary);outline-offset:-2px}
.timeline-toggle>span:first-child{font-size:13px;font-weight:500}
.timeline-toggle-meta{display:inline-flex;align-items:center;gap:8px;color:var(--ops-muted);font-size:11px}
.timeline-toggle-meta small{color:var(--ops-subtle);font-size:11px;font-weight:400}
.timeline-toggle-meta .el-icon{transition:transform .2s ease}
.timeline-toggle-meta .el-icon.expanded{transform:rotate(180deg)}
.timeline-content{min-height:72px;padding:0 16px 4px;border-top:1px solid var(--ops-border-soft)}
.workbench{min-height:0}
.center-tabs :deep(.alert-section),.rule-section{min-height:0}
.center-tabs :deep(.table-wrap){min-height:0}
.alert-timeline :deep(.el-timeline){margin:16px 0 0;padding-left:5px}
.alert-timeline :deep(.el-timeline-item){padding-bottom:18px}
.alert-timeline :deep(.el-timeline-item:last-child){padding-bottom:8px}
.alert-timeline :deep(.el-timeline-item__timestamp){margin-bottom:5px;color:var(--ops-subtle);font-size:11px;line-height:1.4}
.alert-timeline :deep(.el-timeline-item__node){box-shadow:0 0 0 4px #fff}
.timeline-event-title{color:#475467;font-size:12px;font-weight:400;line-height:1.55}
.timeline-event-reason{margin-top:3px;color:#8a93a3;font-size:11px;font-weight:400;line-height:1.55}
.alert-result pre{height:560px;max-height:560px;margin-top:10px;padding:16px;font-size:12px;line-height:1.7}
.center-tabs :deep(.duration-value){display:flex;min-width:0;align-items:center;gap:6px;color:#262626;font:600 12px ui-monospace,SFMono-Regular,Menlo,monospace}
.center-tabs :deep(.duration-icon){flex:0 0 auto;color:#8c8c8c;font-size:12px}
.center-tabs :deep(.duration-segments){display:flex;width:100%;gap:2px}
.center-tabs :deep(.duration-segment){width:10px;height:8px;flex:0 0 10px;background:#ebeef3;border-radius:2px;opacity:.65;transition:background-color .3s,box-shadow .3s,opacity .3s}
.center-tabs :deep(.duration-segment.is-filled){opacity:1}
.center-tabs :deep(.duration-segment:nth-child(1).is-filled){background:#ffff00;box-shadow:0 0 3px #ffff00}
.center-tabs :deep(.duration-segment:nth-child(2).is-filled){background:#ffeb00;box-shadow:0 0 3px #ffeb00}
.center-tabs :deep(.duration-segment:nth-child(3).is-filled){background:#ffd700;box-shadow:0 0 3px #ffd700}
.center-tabs :deep(.duration-segment:nth-child(4).is-filled){background:#ffc300;box-shadow:0 0 3px #ffc300}
.center-tabs :deep(.duration-segment:nth-child(5).is-filled){background:#ffaf00;box-shadow:0 0 3px #ffaf00}
.center-tabs :deep(.duration-segment:nth-child(n+6).is-filled){background:var(--ops-danger);box-shadow:0 0 3px rgba(245,108,108,.75)}
.center-tabs :deep(.alert-time-pair){display:flex;flex-direction:column;gap:4px;font-size:13px;line-height:1.4}
.center-tabs :deep(.alert-time-pair .is-empty){color:#a8abb2}

.grouping-timing-grid,.label-match-fields { grid-template-columns:minmax(0,1fr); }
</style>
