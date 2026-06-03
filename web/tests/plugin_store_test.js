const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function escapeHTML(value) {
    return String(value)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;');
}

function createElement(tagName) {
    return {
        tagName,
        value: '',
        checked: false,
        className: '',
        textContent: '',
        innerHTML: '',
        classList: {
            add() {},
            remove() {}
        }
    };
}

function createHarness(options = {}) {
    const elements = new Map();
    const plugin = Object.assign({
        id: 'nuclei',
        name: 'Nuclei',
        version: '1.0.0',
        source_name: 'official',
        description: 'Template based scanner',
        tags: ['scanner'],
        runtime: { install: { type: 'github_release', repo: 'projectdiscovery/nuclei' } }
    }, options.plugin || {});
    const document = {
        createElement(tagName) {
            const el = createElement(tagName);
            Object.defineProperty(el, 'innerHTML', {
                get() {
                    return escapeHTML(el.textContent || '');
                },
                set(value) {
                    el.textContent = value;
                }
            });
            return el;
        },
        getElementById(id) {
            if (!elements.has(id)) {
                elements.set(id, createElement('div'));
            }
            return elements.get(id);
        }
    };

    const installResponse = options.installResponse || new Promise(() => {});
    const requests = [];
    const context = {
        console,
        document,
        window: {},
        apiFetch(pathname, options) {
            requests.push({ pathname, options: options || {} });
            if (pathname === '/api/plugin-store/install') {
                return installResponse;
            }
            const payloads = {
                '/api/plugin-store/settings': { github_token_configured: true },
                '/api/plugin-store/sources': { sources: [{ name: 'official', url: 'https://github.test/repo.git', updated_at: '2026-06-03T00:00:00Z' }] },
                '/api/plugin-store/installed': { installed: [] },
                '/api/plugin-store/catalog': {
                    catalogs: [{
                        repository: { name: 'CyberStrikeAI Plugins' },
                        plugins: [plugin]
                    }]
                }
            };
            return Promise.resolve({
                ok: true,
                json: () => Promise.resolve(payloads[pathname] || {})
            });
        }
    };
    context.window.showNotification = () => {};
    context.window.refreshMentionTools = () => {};
    context.window.document = document;
    context.globalThis = context;
    vm.createContext(context);
    const script = fs.readFileSync(path.join(__dirname, '../static/js/plugin_store.js'), 'utf8');
    vm.runInContext(script, context, { filename: 'plugin_store.js' });
    return { context, elements, requests };
}

test('install click renders immediate in-row feedback before request finishes', async () => {
    const { context, elements, requests } = createHarness();

    await context.window.loadPluginStorePage();
    assert.match(elements.get('plugin-store-catalog').innerHTML, />安装<\/button>/);
    assert.match(
        elements.get('plugin-store-catalog').innerHTML,
        /onclick="installPluginFromStore\(&quot;official&quot;, &quot;nuclei&quot;\)"/
    );

    context.window.installPluginFromStore('official', 'nuclei');
    await Promise.resolve();

    assert.equal(requests.some(req => req.pathname === '/api/plugin-store/install'), true);
    assert.match(elements.get('plugin-store-catalog').innerHTML, /安装中/);
    assert.match(elements.get('plugin-store-catalog').innerHTML, /正在下载或安装插件运行时/);
});

test('install failure leaves in-row error feedback', async () => {
    const { context, elements } = createHarness({
        installResponse: Promise.resolve({
            ok: false,
            statusText: 'Bad Request',
            json: () => Promise.resolve({ error: 'tool name conflict' })
        })
    });

    await context.window.loadPluginStorePage();
    await context.window.installPluginFromStore('official', 'nuclei');

    assert.match(elements.get('plugin-store-catalog').innerHTML, /安装失败：tool name conflict/);
});

test('conflicting catalog plugin is shown as already available', async () => {
    const { context, elements, requests } = createHarness({
        plugin: {
            install_state: 'conflict',
            conflict_tools: ['nuclei']
        }
    });

    await context.window.loadPluginStorePage();

    const catalogHTML = elements.get('plugin-store-catalog').innerHTML;
    assert.match(catalogHTML, />已存在<\/button>/);
    assert.match(catalogHTML, /工具已存在：nuclei/);
    assert.doesNotMatch(catalogHTML, />安装<\/button>/);

    await context.window.installPluginFromStore('official', 'nuclei');

    assert.equal(requests.some(req => req.pathname === '/api/plugin-store/install'), false);
});
