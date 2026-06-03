(function () {
    const DEFAULT_PLUGIN_SOURCE_NAME = 'official';
    const DEFAULT_PLUGIN_SOURCE_URL = 'https://github.com/MyuriKanao/CyberStrikeAI-Plugins.git';

    const state = {
        settings: null,
        sources: [],
        catalogs: [],
        installed: []
    };

    function esc(value) {
        const div = document.createElement('div');
        div.textContent = value == null ? '' : String(value);
        return div.innerHTML;
    }

    function jsArg(value) {
        return esc(JSON.stringify(value == null ? '' : String(value)));
    }

    function setPluginStoreStatus(message, type) {
        const el = document.getElementById('plugin-store-status');
        if (!el) return;
        el.textContent = message || '';
        el.className = 'plugin-store-status';
        if (type) el.classList.add('is-' + type);
    }

    function notifyPluginStore(message, type) {
        setPluginStoreStatus(message, type);
        if (typeof window.showNotification === 'function') {
            window.showNotification(message, type || 'info');
        }
    }

    async function readPluginStoreJSON(response) {
        const data = await response.json().catch(() => ({}));
        if (!response.ok) {
            throw new Error(data.error || response.statusText || '请求失败');
        }
        return data;
    }

    async function pluginStoreRequest(path, options) {
        const response = await apiFetch(path, options);
        return readPluginStoreJSON(response);
    }

    function installSourceSummary(plugin) {
        const install = plugin && plugin.runtime && plugin.runtime.install ? plugin.runtime.install : {};
        const type = install.type || 'none';
        if (type === 'github_release') {
            const parts = [install.repo, install.version, install.asset].filter(Boolean);
            return { type, detail: parts.join(' / ') || 'GitHub Release' };
        }
        if (type === 'python_venv') {
            const packages = Array.isArray(install.packages) ? install.packages.slice() : [];
            if (install.package) {
                packages.push(install.version ? install.package + '==' + install.version : install.package);
            }
            return { type, detail: packages.join(', ') || 'Python venv' };
        }
        return { type, detail: '无需下载运行时' };
    }

    function installedPluginMap() {
        const out = new Map();
        for (const item of state.installed || []) {
            if (item && item.id) out.set(item.id, item);
        }
        return out;
    }

    function hydratePluginStoreForm() {
        const nameEl = document.getElementById('plugin-store-source-name');
        const urlEl = document.getElementById('plugin-store-source-url');
        const tokenEl = document.getElementById('plugin-store-github-token');
        const clearEl = document.getElementById('plugin-store-clear-token');
        const tokenStatus = document.getElementById('plugin-store-token-status');

        const firstSource = state.sources && state.sources.length > 0 ? state.sources[0] : null;
        if (nameEl && !nameEl.value.trim()) {
            nameEl.value = firstSource ? firstSource.name : DEFAULT_PLUGIN_SOURCE_NAME;
        }
        if (urlEl && !urlEl.value.trim()) {
            urlEl.value = firstSource ? firstSource.url : DEFAULT_PLUGIN_SOURCE_URL;
        }
        if (tokenEl) tokenEl.value = '';
        if (clearEl) clearEl.checked = false;
        if (tokenStatus) {
            const configured = state.settings && state.settings.github_token_configured === true;
            tokenStatus.textContent = configured ? 'Token 状态：已配置' : 'Token 状态：未配置';
            tokenStatus.className = 'plugin-store-token-status ' + (configured ? 'is-configured' : 'is-empty');
        }
    }

    function renderPluginStoreSources() {
        const root = document.getElementById('plugin-store-sources');
        if (!root) return;
        if (!state.sources || state.sources.length === 0) {
            root.innerHTML = '<div class="plugin-store-empty">暂无插件源</div>';
            return;
        }
        root.innerHTML = state.sources.map((source, index) => {
            const updated = source.updated_at ? new Date(source.updated_at).toLocaleString() : '-';
            return `
                <div class="plugin-store-row">
                    <div class="plugin-store-row-main">
                        <div class="plugin-store-row-title">${esc(source.name)}</div>
                        <div class="plugin-store-row-meta">${esc(source.url)}</div>
                        <div class="plugin-store-row-sub">更新时间：${esc(updated)}</div>
                    </div>
                    <div class="plugin-store-row-actions">
                        <button class="btn-small" type="button" onclick="selectPluginStoreSource(${index})">选择</button>
                        <button class="btn-small" type="button" onclick="syncPluginStoreSource(${index})">同步</button>
                    </div>
                </div>`;
        }).join('');
    }

    function renderPluginStoreCatalog() {
        const root = document.getElementById('plugin-store-catalog');
        if (!root) return;
        const installed = installedPluginMap();
        const rows = [];
        for (const catalog of state.catalogs || []) {
            const plugins = Array.isArray(catalog.plugins) ? catalog.plugins : [];
            for (const plugin of plugins) {
                const sourceName = plugin.source_name || (catalog.repository && catalog.repository.name) || '';
                const install = installSourceSummary(plugin);
                const isInstalled = installed.has(plugin.id);
                const tags = Array.isArray(plugin.tags) ? plugin.tags.slice(0, 5) : [];
                rows.push(`
                    <div class="plugin-store-plugin">
                        <div class="plugin-store-plugin-head">
                            <div>
                                <h4>${esc(plugin.name || plugin.id)}</h4>
                                <div class="plugin-store-plugin-id">${esc(plugin.id)} · ${esc(plugin.version || '-')} · ${esc(sourceName)}</div>
                            </div>
                            <button class="btn-small ${isInstalled ? 'btn-secondary' : 'btn-primary'}" type="button" onclick="installPluginFromStore(${jsArg(sourceName)}, ${jsArg(plugin.id)})" ${isInstalled ? 'disabled' : ''}>${isInstalled ? '已安装' : '安装'}</button>
                        </div>
                        <p class="plugin-store-plugin-desc">${esc(plugin.description || '')}</p>
                        <div class="plugin-store-plugin-meta">
                            <span>安装方式：${esc(install.type)}</span>
                            <span>上游来源：${esc(install.detail)}</span>
                        </div>
                        ${tags.length ? '<div class="plugin-store-tags">' + tags.map(tag => `<span>${esc(tag)}</span>`).join('') + '</div>' : ''}
                    </div>`);
            }
        }
        root.innerHTML = rows.length ? rows.join('') : '<div class="plugin-store-empty">暂无可安装插件，请先同步插件源</div>';
    }

    function renderPluginStoreInstalled() {
        const root = document.getElementById('plugin-store-installed');
        if (!root) return;
        if (!state.installed || state.installed.length === 0) {
            root.innerHTML = '<div class="plugin-store-empty">暂无已安装插件</div>';
            return;
        }
        root.innerHTML = state.installed.map(item => {
            const tools = Array.isArray(item.tool_names) && item.tool_names.length ? item.tool_names.join(', ') : '-';
            const updated = item.updated_at ? new Date(item.updated_at).toLocaleString() : '-';
            return `
                <div class="plugin-store-row">
                    <div class="plugin-store-row-main">
                        <div class="plugin-store-row-title">${esc(item.name || item.id)}</div>
                        <div class="plugin-store-row-meta">${esc(item.id)} · ${esc(item.version || '-')} · ${item.enabled ? '已启用' : '未启用'}</div>
                        <div class="plugin-store-row-sub">工具：${esc(tools)}</div>
                        <div class="plugin-store-row-sub">更新时间：${esc(updated)}</div>
                    </div>
                </div>`;
        }).join('');
    }

    function renderPluginStorePage() {
        hydratePluginStoreForm();
        renderPluginStoreSources();
        renderPluginStoreCatalog();
        renderPluginStoreInstalled();
    }

    async function loadPluginStorePage() {
        setPluginStoreStatus('加载中...', 'info');
        const [settings, sources, installed, catalogs] = await Promise.all([
            pluginStoreRequest('/api/plugin-store/settings'),
            pluginStoreRequest('/api/plugin-store/sources'),
            pluginStoreRequest('/api/plugin-store/installed'),
            pluginStoreRequest('/api/plugin-store/catalog').catch(() => ({ catalogs: [] }))
        ]);
        state.settings = settings || {};
        state.sources = sources.sources || [];
        state.installed = installed.installed || [];
        state.catalogs = catalogs.catalogs || [];
        renderPluginStorePage();
        setPluginStoreStatus('已刷新', 'success');
    }

    async function savePluginStoreSettings() {
        const tokenEl = document.getElementById('plugin-store-github-token');
        const clearEl = document.getElementById('plugin-store-clear-token');
        const token = tokenEl ? tokenEl.value.trim() : '';
        const clear = clearEl ? clearEl.checked === true : false;
        if (!token && !clear) {
            notifyPluginStore('Token 未变更', 'info');
            return;
        }
        const settings = await pluginStoreRequest('/api/plugin-store/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                github_token: token,
                clear_github_token: clear
            })
        });
        state.settings = settings || {};
        hydratePluginStoreForm();
        notifyPluginStore('Token 设置已保存', 'success');
    }

    function selectPluginStoreSource(index) {
        const source = state.sources[index];
        if (!source) return;
        const nameEl = document.getElementById('plugin-store-source-name');
        const urlEl = document.getElementById('plugin-store-source-url');
        if (nameEl) nameEl.value = source.name || '';
        if (urlEl) urlEl.value = source.url || '';
        setPluginStoreStatus('已选择源：' + (source.name || ''), 'info');
    }

    async function syncPluginStoreSource(index) {
        if (typeof index === 'number') {
            selectPluginStoreSource(index);
        }
        const name = document.getElementById('plugin-store-source-name')?.value.trim() || DEFAULT_PLUGIN_SOURCE_NAME;
        const url = document.getElementById('plugin-store-source-url')?.value.trim();
        if (!url) {
            notifyPluginStore('请填写 Git 地址', 'error');
            return;
        }
        setPluginStoreStatus('正在同步源...', 'info');
        await pluginStoreRequest('/api/plugin-store/sources', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, url })
        });
        await loadPluginStorePage();
        notifyPluginStore('插件源已同步', 'success');
    }

    async function installPluginFromStore(sourceName, pluginID) {
        if (!sourceName || !pluginID) {
            notifyPluginStore('插件信息不完整', 'error');
            return;
        }
        setPluginStoreStatus('正在安装插件 ' + pluginID + '...', 'info');
        const result = await pluginStoreRequest('/api/plugin-store/install', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ source: sourceName, plugin_id: pluginID })
        });
        await loadPluginStorePage();
        if (typeof window.refreshMentionTools === 'function') {
            window.refreshMentionTools();
        }
        const skipped = Array.isArray(result.skipped) ? result.skipped : [];
        if (skipped.length > 0) {
            notifyPluginStore('插件已安装，但以下工具名冲突未注册：' + skipped.join(', '), 'error');
            return;
        }
        notifyPluginStore('插件已安装并注册工具：' + pluginID, 'success');
    }

    async function reloadPluginTools() {
        setPluginStoreStatus('正在重载插件工具...', 'info');
        const result = await pluginStoreRequest('/api/plugin-store/reload', { method: 'POST' });
        if (typeof loadToolsList === 'function' && typeof toolsPagination !== 'undefined') {
            loadToolsList(toolsPagination.page || 1, '').catch(() => {});
        }
        if (typeof window.refreshMentionTools === 'function') {
            window.refreshMentionTools();
        }
        const skipped = Array.isArray(result.skipped) ? result.skipped : [];
        if (skipped.length > 0) {
            notifyPluginStore('插件工具已重载，但以下工具名冲突未注册：' + skipped.join(', '), 'error');
            return;
        }
        notifyPluginStore('插件工具已重载', 'success');
    }

    window.loadPluginStorePage = loadPluginStorePage;
    window.savePluginStoreSettings = function () {
        savePluginStoreSettings().catch(err => notifyPluginStore('保存失败：' + err.message, 'error'));
    };
    window.selectPluginStoreSource = selectPluginStoreSource;
    window.syncPluginStoreSource = function (index) {
        syncPluginStoreSource(index).catch(err => notifyPluginStore('同步失败：' + err.message, 'error'));
    };
    window.installPluginFromStore = function (sourceName, pluginID) {
        installPluginFromStore(sourceName, pluginID).catch(err => notifyPluginStore('安装失败：' + err.message, 'error'));
    };
    window.reloadPluginTools = function () {
        reloadPluginTools().catch(err => notifyPluginStore('重载失败：' + err.message, 'error'));
    };
})();
