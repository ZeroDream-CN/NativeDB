const API_BASE              = document.querySelector('meta[name="api-base"]')?.content || window.location.origin;
let   allNatives            = [];
let   currentNativeHash     = null;
let   currentNativeParams   = [];
let   currentNativeExamples = {};
let   authToken             = localStorage.getItem('native_db_token') || null;
let   currentUser           = null;
let   transEditor           = null;
let   codeEditor            = null;
let   monacoLoaded          = false;
let   renderQueue           = [];
let   renderedCount         = 0;
const BATCH_SIZE            = 100;

$(document).ready(function () {
    marked.use({ gfm: true, breaks: true });
    $('.loading-overlay').fadeOut();

    checkLoginStatus();
    fetchNatives();
    initMonaco();

    let debounceTimer;
    $('#search-input').on('input', function () {
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => filterData(), 300);
    });
    $('#filter-apiset, #filter-namespace').on('change', filterData);

    $('#natives-list').on('scroll', function () {
        const $this = $(this);
        if ($this.scrollTop() + $this.innerHeight() >= $this[0].scrollHeight - 200) {
            renderNextBatch();
        }
    });

    $(window).on('hashchange', function () {
        const hashFromURL = window.location.hash.slice(2);
        if (hashFromURL) {
            selectNative(hashFromURL);
        }
    });
    
    // 打开登录框
    $('#btn-login').click(() => {
        $('#modal-login').removeClass('hidden');
        $('#input-username').focus();
    });

    // 点击头像打开个人信息
    $('#user-info').click(() => {
        if(currentUser) {
            $('#profile-avatar').attr('src', currentUser.avatar);
            $('#profile-name').text(currentUser.username);
            $('#profile-email').text(currentUser.email);
            $('#modal-user').removeClass('hidden');
        }
    });

    // 打开修改密码模态框
    $('#btn-change-pass').click(() => {
        $('#modal-user').addClass('hidden');
        $('#modal-change-pass').removeClass('hidden');
        $('#input-old-pass').val('');
        $('#input-new-pass').val('');
        $('#input-confirm-pass').val('');
        $('#input-old-pass').focus();
    });

    // 提交修改密码
    $('#btn-submit-change-pass').click(submitChangePassword);

    // 提交登录
    $('#btn-submit-login').click(doLogin);
    $('#input-password').keypress(function (e) {
        if (e.which == 13) doLogin();
    });

    // 打开翻译编辑
    $('#btn-edit-trans').click(() => {
        const currentDesc = allNatives.find(n => n.hash === currentNativeHash)?.description_cn || "";
        $('#modal-trans').removeClass('hidden');
        if (transEditor) {
            transEditor.setValue(currentDesc);
            setTimeout(() => transEditor.layout(), 50);
        } else {
            console.warn('Monaco editor not ready yet');
            $('#input-trans-cn').val(currentDesc);
        }
    });

    // 提交翻译
    $('#btn-submit-trans').click(submitTranslation);

    // 打开代码编辑
    $('#btn-edit-code').click(() => {
        $('#modal-code').removeClass('hidden');

        if (codeEditor) {
            const lang = $('#input-code-lang').val();
            updateCodeEditorLang(lang);
            const existingCode = currentNativeExamples[lang] || '';
            codeEditor.setValue(existingCode);
            setTimeout(() => codeEditor.layout(), 50);
        }
    });

    // 语言改变
    $('#input-code-lang').on('change', function () {
        if (codeEditor) {
            const lang = $(this).val();
            updateCodeEditorLang(lang);
            const existingCode = currentNativeExamples[lang] || '';
            codeEditor.setValue(existingCode);
        }
    });

    // 提交代码
    $('#btn-submit-code').click(submitExample);

    // 打开参数编辑
    $('#btn-edit-params').click(() => {
        openParamsEditor();
    });

    // 提交参数
    $('#btn-submit-params').click(submitParams);

    // 把页面滚动到顶部
    window.scrollTo(0, 0);
});

function initMonaco() {
    if (typeof require === 'undefined') {
        console.error("Monaco loader not found.");
        return;
    }

    require.config({ paths: { 'vs': 'https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.45.0/min/vs' } });
    require(['vs/editor/editor.main'], function () {
        monacoLoaded = true;
        const transContainer = document.getElementById('editor-trans-cn');
        if (transContainer) {
            transEditor = monaco.editor.create(transContainer, {
                value: '',
                language: 'markdown',
                theme: 'vs-dark',
                automaticLayout: true,
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                wordWrap: 'on',
                fontSize: 14,
                fontFamily: "'JetBrains Mono', Consolas, monospace"
            });
        }

        const codeContainer = document.getElementById('editor-code-content');
        if (codeContainer) {
            codeEditor = monaco.editor.create(codeContainer, {
                value: '',
                language: 'lua',
                theme: 'vs-dark',
                automaticLayout: true,
                minimap: { enabled: true },
                scrollBeyondLastLine: false,
                fontSize: 14,
                fontFamily: "'JetBrains Mono', Consolas, monospace"
            });
        }
    });
}

function updateCodeEditorLang(langValue) {
    if (!codeEditor || !monacoLoaded) return;

    let monacoLang = 'lua';
    switch (langValue) {
        case 'c#': monacoLang = 'csharp'; break;
        case 'javascript': monacoLang = 'javascript'; break;
        default: monacoLang = 'lua';
    }

    monaco.editor.setModelLanguage(codeEditor.getModel(), monacoLang);
}

function getAuthHeader() {
    if (!authToken) return {};
    return { 'Authorization': `Bearer ${authToken}` };
}

async function checkLoginStatus() {
    if (!authToken) {
        updateAuthUI(false);
        return;
    }

    try {
        const res = await fetch(`${API_BASE}/api/auth/me`, {
            headers: getAuthHeader()
        });

        if (res.ok) {
            currentUser = await res.json();
            updateAuthUI(true);
        } else {
            // Token 失效
            logout(false); 
        }
    } catch (e) {
        console.error("Auth check failed", e);
        updateAuthUI(false);
    }
}

function updateAuthUI(isLoggedIn) {
    if (isLoggedIn && currentUser) {
        $('#btn-login').addClass('hidden');
        $('#user-info').removeClass('hidden').addClass('flex');
        $('#user-name').text(currentUser.username);
        $('#user-avatar').attr('src', currentUser.avatar);

        // 显示编辑按钮
        $('#btn-edit-trans').removeClass('hidden');
        $('#btn-edit-code').removeClass('hidden');
        $('#btn-edit-params').removeClass('hidden');
    } else {
        $('#btn-login').removeClass('hidden');
        $('#user-info').addClass('hidden').removeClass('flex');

        // 隐藏编辑按钮
        $('#btn-edit-trans').addClass('hidden');
        $('#btn-edit-code').addClass('hidden');
        $('#btn-edit-params').addClass('hidden');
    }
}

async function doLogin() {
    const username = $('#input-username').val().trim();
    const password = $('#input-password').val().trim();

    if (!username || !password) {
        Swal.fire({
            icon: 'error',
            title: '错误',
            theme: "material-ui-dark",
            text: '请输入用户名和密码'
        });
        return;
    }

    try {
        const res = await fetch(`${API_BASE}/api/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });

        const data = await res.json();

        if (res.ok) {
            authToken = data.token;
            currentUser = data.user;
            localStorage.setItem('native_db_token', authToken);
            
            $('#modal-login').addClass('hidden');
            $('#input-username').val('');
            $('#input-password').val('');
            
            updateAuthUI(true);
            sendNotification('success', `欢迎回来, ${currentUser.username}`);
        } else {
            Swal.fire({
                icon: 'error',
                title: '登录失败',
                theme: "material-ui-dark",
                text: data.error || '用户名或密码错误'
            });
        }
    } catch (e) {
        Swal.fire({
            icon: 'error',
            title: '错误',
            theme: "material-ui-dark",
            text: '网络请求出错: ' + e.message
        });
    }
}

async function submitChangePassword() {
    const oldPass = $('#input-old-pass').val();
    const newPass = $('#input-new-pass').val();
    const confirmPass = $('#input-confirm-pass').val();

    if (!oldPass || !newPass || !confirmPass) {
        Swal.fire({ icon: 'error', title: '错误', text: '请填写所有字段' });
        return;
    }

    if (newPass !== confirmPass) {
        Swal.fire({ icon: 'error', title: '错误', text: '两次输入的新密码不一致' });
        return;
    }

    if (newPass.length < 6) {
        Swal.fire({ icon: 'error', title: '错误', text: '新密码长度至少需要6位' });
        return;
    }

    try {
        const res = await fetch(`${API_BASE}/api/auth/change-password`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', ...getAuthHeader() },
            body: JSON.stringify({ old_password: oldPass, new_password: newPass })
        });

        const data = await res.json();
        if (res.ok) {
            Swal.fire({ icon: 'success', title: '成功', text: '密码修改成功' });
            $('#modal-change-pass').addClass('hidden');
        } else {
            Swal.fire({ icon: 'error', title: '错误', text: data.error || '修改失败' });
        }
    } catch (e) {
        Swal.fire({ icon: 'error', title: '错误', text: '网络请求失败: ' + e.message });
    }
}

function logout(reload = true) {
    authToken = null;
    currentUser = null;
    localStorage.removeItem('native_db_token');
    if (reload) location.reload();
    else updateAuthUI(false);
}

function sendNotification(icon, text, cb) {
    Swal.fire({
        icon: icon,
        theme: "material-ui-dark",
        text: text,
        showConfirmButton: false,
        timer: 2000,
        position: 'top-end',
    }).then(() => {
        if (cb) cb();
    });
}

async function fetchNatives() {
    try {
        const res = await fetch(`${API_BASE}/api/natives`);
        if (!res.ok) throw new Error("API Error");

        allNatives = await res.json();

        $('#status-bar').text(`已加载 ${allNatives.length} 个函数`);
        initFilters();
        filterData();
        const hashFromURL = window.location.hash.slice(2);
        if (hashFromURL) {
            selectNative(hashFromURL);
        }
    } catch (err) {
        $('#natives-list').html(`<div class="p-10 text-center text-red-500">加载函数失败<br>${err.message}</div>`);
    }
}

async function selectNative(hash) {
    currentNativeHash = hash;
    currentNativeExamples = {};

    $('.native-row').removeClass('active');
    $(`#row-${hash}`).addClass('active');
    $('#empty-state').removeClass('md:flex').addClass('hidden');
    $('#detail-container').removeClass('hidden');

    $('#detail-view').removeClass('translate-x-full');
    if (window.innerWidth < 768) {
        $('body').addClass('overflow-hidden');
    }

    const basicInfo = allNatives.find(n => n.hash === hash);
    if (basicInfo) renderDetailBasic(basicInfo);

    try {
        const res = await fetch(`${API_BASE}/api/native/${hash}`);
        const json = await res.json();

        if (json.data) {
            if (basicInfo) {
                basicInfo.description_cn = json.data.description_cn;
                basicInfo.params = json.data.params;
            }

            renderDetailFull(json.data, json.source_available);
            window.location.hash = `_${hash}`;
        }
    } catch (e) {
        console.error("Fetch detail failed", e);
    }
}

function closeMobileDetail() {
    $('#detail-view').addClass('translate-x-full');
    $('body').removeClass('overflow-hidden');
    $('.native-row').removeClass('active');
    currentNativeHash = null;
    currentNativeExamples = {};
    window.location.hash = '';
    window.history.replaceState({}, document.title, window.location.pathname);
}

function renderDetailBasic(n) {
    currentNativeParams = n.params || [];

    $('#detail-name').text(n.name || n.hash);
    $('#detail-namespace').text(n.namespace);
    $('#detail-apiset').text(getApisetName(n.apiset)).attr('class', `px-2 py-0.5 rounded text-xs font-semibold border ${getApisetColor(n.apiset)}`);
    $('#detail-hash').text(n.hash);

    if (n.jhash) {
        $('#detail-jhash').removeClass('hidden');
        $('#detail-jhash').text(n.jhash);
    } else {
        $('#detail-jhash').addClass('hidden');
    }

    const paramsStr = n.params.map(p => `${p.type} ${p.name}`).join(', ');
    const scriptName = n.name ? toCamelCase(n.name) : n.hash;
    const defText = `// ${scriptName}\n${n.return_type} ${n.name || n.hash} (${paramsStr})`;
    const $defCode = $('#detail-definition');
    $defCode.text(defText);
    Prism.highlightElement($defCode[0]);

    renderParams(n.params);

    $('#detail-desc-cn').html('<span class="text-gray-600 italic">加载中...</span>');
    $('#detail-desc-en').html('');

    switchTab('example');
    $('#source-code-viewer').text("// 点击 '底层源码' 标签以加载函数的底层代码。");
}

function toCamelCase(str) {
    return str
        .toLowerCase()
        .split('_')
        .map(word => word.charAt(0).toUpperCase() + word.slice(1))
        .join('');
}

function renderParams(params) {
    const $paramsList = $('#detail-params-list');
    $paramsList.empty();

    params.forEach(p => {
        let description = p.description_cn || p.description || '';
        let parsed = marked.parse(description).replace(/<p>/g, '').replace(/<\/p>/g, '');
        const paramHtml = `
                    <li class="param-item">
                        <span class="text-zinc-400 mr-1 param-type type-${p.type}">${p.type}</span>
                        <span class="text-white font-bold mr-1 param-name">${p.name}:</span>
                        <span class="param-desc">${parsed}</span>
                    </li>
                `;
        $paramsList.append(paramHtml);
    });

    if (params.length === 0) {
        $('#detail-params-list').html('<li class="text-gray-500 italic">此函数没有参数。</li>');
    }
}

function renderDetailFull(data, hasSource) {
    currentNativeParams = data.params || [];
    renderParams(currentNativeParams);

    if (data.description_cn && data.description_cn.trim() !== "") {
        $('#detail-desc-cn').html(marked.parse(data.description_cn));
    } else if (data.description_original) {
        $('#detail-desc-cn').html(marked.parse(data.description_original));
    } else {
        $('#detail-desc-cn').html('<span class="text-gray-600">暂无描述信息。</span>');
    }

    if (data.description_original) {
        $('#detail-desc-en').html(marked.parse(data.description_original));
    }

    $('#detail-desc-cn pre code, #detail-desc-en pre code').each((i, b) => Prism.highlightElement(b));

    $('#detail-desc-en a, #detail-desc-cn a').each((i, a) => {
        const href = $(a).attr('href');
        if (!href.startsWith('#') && !href.startsWith('/')) {
            $(a).attr('target', '_blank');
        }
    });

    $('#tab-btn-source').attr('data-has-source', hasSource ? 'true' : 'false');
    if (!hasSource) {
        $('#source-code-viewer').text("-- 数据库中暂无该函数的底层代码。");
    } else {
        $('#source-code-viewer').text("// 点击 '底层源码' 标签以加载函数的底层代码。");
    }
    
    if (authToken && currentUser) updateAuthUI(true);
}

function formatCode(sourceCode) {
    let formatted = js_beautify(sourceCode, {
        indent_size: 4,
        indent_char: ' ',
        brace_style: "collapse",
        preserve_newlines: true,
        e4x: true
    });
    return formatted.replaceAll(" - > ", "->");
}

function cleanupCode(sourceCode) {
    return sourceCode
        .split('\n')
        .map(line => line.trim())
        .join('\n');
}

async function loadSourceCode(hash) {
    const $container = $('#source-code-viewer');
    $container.text("// 正在加载中，请稍候...");

    try {
        const res = await fetch(`${API_BASE}/api/native/${hash}/source`);
        if (res.status === 404) {
            $container.text("// 数据库中暂无该函数的底层代码。");
            return;
        }
        const data = await res.json();
        const codeContent = cleanupCode(data.content);
        $container.text(formatCode(codeContent));
        Prism.highlightElement($container[0]);
    } catch (e) {
        $container.text("// 加载底层代码时发生错误。");
    }
}

async function loadExampleCode(hash) {
    const $container = $('#example-code-viewer');
    $container.text("// 正在加载中，请稍候...");
    currentNativeExamples = {};

    try {
        const res = await fetch(`${API_BASE}/api/native/${hash}/example`);
        if (res.status === 404) {
            $container.text("-- 数据库中暂无该函数的示例代码。");
            return;
        }
        const data = await res.json();

        if (Array.isArray(data)) {
            data.forEach(item => {
                if (item.language && item.code) {
                    currentNativeExamples[item.language.toLowerCase()] = item.code;
                }
            });
        }

        $container.text(data?.[0]?.code || "-- 数据库中暂无该函数的示例代码。");
        const lang = data?.[0]?.language || 'lua';
        $container.removeClass().addClass(`language-${lang}`);
        Prism.highlightElement($container[0]);
    } catch (e) {
        $container.text("// 加载示例代码时发生错误。");
        currentNativeExamples = {};
    }
}

async function submitTranslation() {
    if (!currentNativeHash) return;
    const content = transEditor ? transEditor.getValue() : $('#input-trans-cn').val();
    const path = `/api/native/${currentNativeHash}/translate`;

    try {
        const res = await fetch(`${API_BASE}${path}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...getAuthHeader()
            },
            body: JSON.stringify({ description_cn: content })
        });

        if (res.ok) {
            sendNotification('success', '翻译保存成功！');
            selectNative(currentNativeHash);
            const n = allNatives.find(x => x.hash === currentNativeHash);
            if (n) n.description_cn = content;
            $('#modal-trans').addClass('hidden');
        } else {
            const err = await res.json();
            Swal.fire({
                icon: 'error',
                title: '错误',
                text: "保存失败: " + (err.error || "未知错误")
            });
        }
    } catch (e) {
        Swal.fire({icon: 'error', title: '错误', text: "请求失败: " + e.message});
    }
}

async function submitExample() {
    if (!currentNativeHash) return;
    const lang = $('#input-code-lang').val();
    const code = codeEditor ? codeEditor.getValue() : $('#input-code-content').val();
    const path = `/api/native/${currentNativeHash}/example`;

    try {
        const res = await fetch(`${API_BASE}${path}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...getAuthHeader()
            },
            body: JSON.stringify({ language: lang, code: code })
        });

        if (res.ok) {
            sendNotification('success', '示例代码保存成功！');
            $('#modal-code').addClass('hidden');
            loadExampleCode(currentNativeHash);
        } else {
            const err = await res.json();
            Swal.fire({icon: 'error', title: '错误', text: "保存失败: " + err.error});
        }
    } catch (e) {
        Swal.fire({icon: 'error', title: '错误', text: "请求失败: " + e.message});
    }
}

function openParamsEditor() {
    const $container = $('#params-edit-container');
    $container.empty();

    if (!currentNativeParams || currentNativeParams.length === 0) {
        $container.html('<div class="text-center text-gray-500 py-4">此函数没有参数，无需翻译。</div>');
    } else {
        currentNativeParams.forEach((p, index) => {
            const originalDesc = p.description || "(无原始描述)";
            const val = p.description_cn || "";

            const html = `
                <div class="bg-gray-900 border border-gray-700 p-3 rounded">
                    <div class="flex items-center gap-2 mb-2">
                        <span class="text-blue-400 font-mono text-xs font-bold">${p.type}</span>
                        <span class="text-white font-mono text-sm font-bold">${p.name}</span>
                    </div>
                    <div class="mb-2 text-xs text-gray-500 bg-gray-950 p-2 rounded border border-gray-800">
                        <span class="font-bold text-gray-600 select-none">Original: </span>
                        ${originalDesc}
                    </div>
                    <input type="text" 
                        class="param-input w-full bg-gray-800 border border-gray-600 rounded px-3 py-2 text-white text-sm focus:outline-none focus:border-indigo-500" 
                        placeholder="输入中文描述..." 
                        data-name="${p.name}" 
                        value="${val.replace(/"/g, '&quot;')}">
                </div>
            `;
            $container.append(html);
        });
    }

    $('#modal-params').removeClass('hidden');
}

async function submitParams() {
    if (!currentNativeHash) return;
    const newParams = [];
    $('.param-input').each(function () {
        const name = $(this).data('name');
        const descCn = $(this).val().trim();
        if (name) {
            newParams.push({
                name: name,
                description_cn: descCn
            });
        }
    });

    const path = `/api/native/${currentNativeHash}/params`;

    try {
        const res = await fetch(`${API_BASE}${path}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...getAuthHeader()
            },
            body: JSON.stringify({ params: newParams })
        });

        if (res.ok) {
            sendNotification('success', '参数翻译保存成功！');
            $('#modal-params').addClass('hidden');
            selectNative(currentNativeHash);
            const n = allNatives.find(x => x.hash === currentNativeHash);
            if (n && n.params) {
                n.params.forEach(p => {
                    const updated = newParams.find(np => np.name === p.name);
                    if (updated) p.description_cn = updated.description_cn;
                });
            }
        } else {
            const err = await res.json();
            Swal.fire({icon: 'error', title: '错误', text: "保存失败: " + err.error});
        }
    } catch (e) {
        Swal.fire({icon: 'error', title: '错误', text: "请求失败: " + e.message});
    }
}

function initFilters() {
    const namespaces = [...new Set(allNatives.map(n => n.namespace))].sort();
    const $nsSelect = $('#filter-namespace');
    $nsSelect.find('option:not([value="all"])').remove();
    namespaces.forEach(ns => $nsSelect.append(`<option value="${ns}">${ns}</option>`));
}

function filterData() {
    const query = $('#search-input').val().toLowerCase();
    const apiFilter = $('#filter-apiset').val();
    const nsFilter = $('#filter-namespace').val();

    let filtered = allNatives.filter(n => {
        const matchText = n.name.toLowerCase().includes(query) || n.hash.toLowerCase().includes(query) || n.name.toLowerCase().replace(/_/g, '').includes(query);
        const matchApi = apiFilter === 'all' || n.apiset === apiFilter;
        const matchNs = nsFilter === 'all' || n.namespace === nsFilter;
        return matchText && matchApi && matchNs;
    });

    filtered.sort((a, b) => {
        if (a.namespace !== b.namespace) return a.namespace.localeCompare(b.namespace);
        return a.name.localeCompare(b.name);
    });

    renderQueue = [];
    let lastNs = null;
    filtered.forEach(native => {
        if (native.namespace !== lastNs) {
            renderQueue.push({ type: 'header', text: native.namespace });
            lastNs = native.namespace;
        }
        renderQueue.push({ type: 'row', data: native });
    });

    const $container = $('#natives-list');
    $container.empty();
    renderedCount = 0;

    if (renderQueue.length === 0) {
        $container.html('<div class="p-10 text-center text-gray-500">暂无符合条件的函数。</div>');
    } else {
        renderNextBatch();
        $container.scrollTop(0);
    }
}

function renderNextBatch() {
    if (renderedCount >= renderQueue.length) return;

    const limit = Math.min(renderedCount + BATCH_SIZE, renderQueue.length);
    let htmlBuffer = '';

    for (let i = renderedCount; i < limit; i++) {
        const item = renderQueue[i];
        if (item.type === 'header') {
            htmlBuffer += `
                <div class="sticky top-0 bg-gray-800/95 backdrop-blur border-b border-t border-gray-700 px-4 py-1.5 text-xs font-bold text-gray-400 uppercase tracking-wider z-0">
                    ${item.text}
                </div>
            `;
        } else {
            const native = item.data;
            const params = native.params || [];
            const paramsHtml = params.map((p, idx) => {
                const isLast = idx === params.length - 1;
                return `<span class="text-gray-400">${getGenericTypeColor(p.type)}${p.type}</span> <span class="text-gray-300 group-hover:text-white">${p.name}</span>${isLast ? '' : ', '}`;
            }).join('');

            const isActive = currentNativeHash === native.hash ? 'active' : '';
            let nativeIcons = '';
            if (native.source_available) {
                nativeIcons += '<i class="fas fa-code text-yellow-400 text-xs ml-1" title="底层源码"></i>';
            }
            if (native.example_available) {
                nativeIcons += '<i class="fas fa-file-alt text-green-400 text-xs ml-1" title="示例代码"></i>';
            }

            htmlBuffer += `
                <div class="native-row cursor-pointer px-4 py-1.5 border-b border-gray-700/50 flex items-baseline gap-3 transition-colors group ${isActive}" 
                        onclick="selectNative('${native.hash}')" id="row-${native.hash}">
                    <span class="text-xs font-mono font-bold shrink-0 w-16 text-right ${getTypeColorClass(native.return_type)}">${native.return_type}</span>
                    <div class="flex-1 overflow-hidden whitespace-nowrap text-ellipsis font-mono text-sm text-gray-300 font-medium truncate">
                        <span class="text-gray-100 group-hover:text-blue-300 transition-colors">${native.name || native.hash}${nativeIcons}</span>
                        <span class="text-gray-500 text-xs ml-1">( ${paramsHtml} )</span>
                    </div>
                </div>
            `;
        }
    }

    $('#natives-list').append(htmlBuffer);
    renderedCount = limit;
}

function switchTab(tab) {
    if (tab === 'example') {
        $('#tab-content-example').removeClass('hidden').addClass('block');
        $('#tab-content-source').addClass('hidden').removeClass('block');
        $('#tab-btn-example').addClass('border-blue-500 text-white').removeClass('border-transparent text-gray-500');
        $('#tab-btn-source').removeClass('border-blue-500 text-white').addClass('border-transparent text-gray-500');
        if (currentNativeHash) {
            loadExampleCode(currentNativeHash);
        }
    } else {
        $('#tab-content-example').addClass('hidden').removeClass('block');
        $('#tab-content-source').removeClass('hidden').addClass('block');
        $('#tab-btn-source').addClass('border-blue-500 text-white').removeClass('border-transparent text-gray-500');
        $('#tab-btn-example').removeClass('border-blue-500 text-white').addClass('border-transparent text-gray-500');

        if (currentNativeHash && $('#tab-btn-source').attr('data-has-source') === 'true') {
            const currentText = $('#source-code-viewer').text();
            if (currentText.startsWith("// 点击")) {
                loadSourceCode(currentNativeHash);
            }
        }
    }
}

function getGenericTypeColor(type) { return ''; }

function getTypeColorClass(type) {
    if (!type) return 'text-gray-400';
    type = type.replace('*', '').toLowerCase();
    if (type === 'void') return 'type-void';
    if (type === 'int' || type === 'hash') return 'type-int';
    if (type === 'float') return 'type-float';
    if (type === 'bool' || type === 'boolean') return 'type-bool';
    if (type === 'vector3') return 'type-vector3';
    if (['ped', 'vehicle', 'entity', 'object'].includes(type)) return 'type-entity';
    return 'text-gray-400';
}

function getApisetColor(apiset) {
    if (apiset === 'client') return 'bg-purple-900/50 text-purple-200 border-purple-700/50';
    if (apiset === 'server') return 'bg-orange-900/50 text-orange-200 border-orange-700/50';
    if (apiset === 'shared') return 'bg-blue-900/50 text-blue-200 border-blue-700/50';
    return 'bg-gray-700 text-gray-300';
}

function getApisetName(apiset) {
    if (apiset === 'client') return '客户端';
    if (apiset === 'server') return '服务器';
    if (apiset === 'shared') return '共享';
    return '未知';
}

function copyToClipboard(element) {
    const text = $(element).text();
    if (navigator.clipboard) {
        navigator.clipboard.writeText(text).then(() => {
            $(element).text('已复制！');
            setTimeout(() => $(element).text(text), 1000);
        });
    } else {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        $(element).text('已复制！');
        setTimeout(() => $(element).text(text), 1000);
    }
}
