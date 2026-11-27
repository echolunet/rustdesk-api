(() => {
  const state = {
    token: localStorage.getItem('rd_admin_token') || '',
    user: null,
    tab: 'dashboard',
    usersSubTab: 'users',
    usersPage: 1,
    usersKeyword: '',
    usersStatusFilter: 'all',
    devicesSubTab: 'devices',
    devicesPage: 1,
    devicesKeyword: '',
    devicesOnlineFilter: 'all',
    devicesGroupFilter: '',
    auditSubTab: 'conn',
    auditPage: 1,
    auditKeyword: '',
    selectedABGuid: '',
    abPeerKeyword: '',
    pendingLogin: null, // { username, password, mode: 'tfa_check'|'email_check', useRecovery: false }
    resendTimer: null,
  };

  const $ = (id) => document.getElementById(id);

  function esc(v) {
    return String(v ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function toast(msg, ok = true) {
    const el = document.createElement('div');
    el.className = 'toast ' + (ok ? 'ok' : 'err');
    el.textContent = (ok ? '✓ ' : '✕ ') + msg;
    $('toast-root').appendChild(el);
    setTimeout(() => el.remove(), 3500);
  }

  function fmtTime(val) {
    if (!val) return '-';
    const d = new Date(val);
    if (Number.isNaN(d.getTime())) return String(val);
    return d.toLocaleString();
  }

  function relTime(val) {
    if (!val) return '-';
    const d = new Date(val);
    if (Number.isNaN(d.getTime())) return String(val);
    const diff = Math.floor((Date.now() - d.getTime()) / 1000);
    if (diff < 60) return '刚刚';
    if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前';
    if (diff < 86400) return Math.floor(diff / 3600) + ' 小时前';
    if (diff < 86400 * 30) return Math.floor(diff / 86400) + ' 天前';
    return d.toLocaleDateString();
  }

  function fmtBytes(bytes) {
    const n = Number(bytes || 0);
    if (!n) return '-';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    return (n / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function argbToHex(colorVal) {
    const n = Number(colorVal);
    if (!Number.isFinite(n) || n <= 0) return '#2563eb';
    return '#' + (n & 0xffffff).toString(16).padStart(6, '0');
  }

  function avatarColor(name) {
    const palette = ['#2563eb', '#7c3aed', '#0891b2', '#059669', '#d97706', '#dc2626', '#4f46e5'];
    let hash = 0;
    const s = String(name || 'u');
    for (let i = 0; i < s.length; i += 1) {
      hash = (hash * 31 + s.charCodeAt(i)) >>> 0;
    }
    return palette[hash % palette.length];
  }

  function osBadge(osName) {
    const s = String(osName || '').trim();
    if (!s) return '<span class="badge badge-gray">未知</span>';
    const low = s.toLowerCase();
    if (low.includes('win')) return `<span class="badge badge-blue">${esc(s)}</span>`;
    if (low.includes('mac') || low.includes('darwin')) return `<span class="badge badge-gray">${esc(s)}</span>`;
    if (low.includes('linux') || low.includes('ubuntu') || low.includes('debian')) return `<span class="badge badge-amber">${esc(s)}</span>`;
    if (low.includes('android') || low.includes('ios')) return `<span class="badge badge-green">${esc(s)}</span>`;
    return `<span class="badge badge-gray">${esc(s)}</span>`;
  }

  function copyText(text, label = '已复制到剪贴板') {
    if (!text) {
      toast('内容为空，无法复制', false);
      return;
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(
        () => toast(label),
        () => fallbackCopy(text, label)
      );
      return;
    }
    fallbackCopy(text, label);
  }

  function fallbackCopy(text, label) {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand('copy');
      toast(label);
    } catch (_) {
      toast('复制失败，请手动复制', false);
    }
    ta.remove();
  }

  function renderSkeleton(rows = 4) {
    const lines = Array.from({ length: rows }, (_, idx) =>
      `<div class="skeleton-line" style="width:${idx % 2 === 0 ? '100%' : '76%'}"></div>`
    ).join('');
    return `<div class="card">${lines}</div>`;
  }

  function renderEmpty(title, desc) {
    return `<div class="empty-state"><strong>${esc(title)}</strong><span>${esc(desc)}</span></div>`;
  }

  function renderPagination(current, pageSize, total) {
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    if (total <= pageSize && current === 1) return '';
    return `
      <div class="pagination-bar">
        <span>共 ${esc(total)} 条 · 第 ${esc(current)} / ${esc(totalPages)} 页</span>
        <div class="row">
          <button class="btn btn-xs btn-secondary" data-page="${current - 1}" ${current <= 1 ? 'disabled' : ''}>上一页</button>
          <button class="btn btn-xs btn-secondary" data-page="${current + 1}" ${current >= totalPages ? 'disabled' : ''}>下一页</button>
        </div>
      </div>
    `;
  }

  // ---------------- Theme & Drawer ----------------
  function applyTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('rd_admin_theme', theme);
    const isDark = theme === 'dark';
    $('icon-theme-sun').classList.toggle('hidden', isDark);
    $('icon-theme-moon').classList.toggle('hidden', !isDark);
  }

  function initTheme() {
    const saved = localStorage.getItem('rd_admin_theme');
    if (saved === 'dark' || saved === 'light') {
      applyTheme(saved);
      return;
    }
    const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    applyTheme(prefersDark ? 'dark' : 'light');
  }

  function openSidebar() {
    $('sidebar').classList.add('open');
    $('sidebar-backdrop').classList.remove('hidden');
  }

  function closeSidebar() {
    $('sidebar').classList.remove('open');
    $('sidebar-backdrop').classList.add('hidden');
  }

  // ---------------- API & Modal ----------------
  async function api(path, options = {}) {
    const headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    if (state.token) {
      headers.Authorization = 'Bearer ' + state.token;
    }
    const resp = await fetch(path, Object.assign({}, options, { headers }));
    let data = {};
    const txt = await resp.text();
    try {
      data = txt ? JSON.parse(txt) : {};
    } catch (_) {
      data = { raw: txt };
    }
    if (resp.status === 401 && path !== '/admin/api/login') {
      logout();
      throw new Error('登录已失效，请重新登录');
    }
    if (!resp.ok) {
      throw new Error(data.error || ('请求失败 HTTP ' + resp.status));
    }
    return data;
  }

  function openModal(title, html, onMount) {
    $('modal-title').textContent = title;
    $('modal-body').innerHTML = html;
    $('modal-backdrop').classList.remove('hidden');
    if (onMount) onMount($('modal-body'));
  }

  function closeModal() {
    $('modal-backdrop').classList.add('hidden');
    $('modal-body').innerHTML = '';
  }

  function confirmDialog({ title, message, confirmText = '确认', danger = true }) {
    return new Promise((resolve) => {
      openModal(
        title || '请确认操作',
        `<div class="stack">
          <p style="margin:0;color:var(--text-secondary);">${esc(message)}</p>
          <div class="row" style="justify-content:flex-end;margin-top:8px;">
            <button type="button" id="confirm-cancel" class="btn btn-secondary">取消</button>
            <button type="button" id="confirm-ok" class="btn ${danger ? 'btn-danger-solid' : 'btn-primary'}">${esc(confirmText)}</button>
          </div>
        </div>`,
        (root) => {
          root.querySelector('#confirm-cancel').onclick = () => {
            closeModal();
            resolve(false);
          };
          root.querySelector('#confirm-ok').onclick = () => {
            closeModal();
            resolve(true);
          };
        }
      );
    });
  }

  function resetLoginSteps() {
    state.pendingLogin = null;
    if (state.resendTimer) {
      clearInterval(state.resendTimer);
      state.resendTimer = null;
    }
    $('login-step-1').classList.remove('hidden');
    $('login-step-2').classList.add('hidden');
    $('login-error').classList.add('hidden');
    $('login-otp-error').classList.add('hidden');
    $('login-recovery-input').value = '';
    document.querySelectorAll('#otp-boxes .otp-box').forEach((inp) => {
      inp.value = '';
    });
  }

  function showLogin() {
    resetLoginSteps();
    $('app-shell').classList.add('hidden');
    $('login-view').classList.remove('hidden');
  }

  function showApp() {
    resetLoginSteps();
    $('login-view').classList.add('hidden');
    $('app-shell').classList.remove('hidden');
    const uname = state.user ? state.user.username : 'admin';
    $('current-admin-name').textContent = uname;
    $('current-admin-avatar').textContent = uname.charAt(0).toUpperCase();
    $('current-admin-avatar').style.background = avatarColor(uname);
  }

  function logout() {
    state.token = '';
    state.user = null;
    localStorage.removeItem('rd_admin_token');
    closeSidebar();
    showLogin();
  }

  async function boot() {
    initTheme();
    if (!state.token) {
      showLogin();
      return;
    }
    try {
      const res = await api('/admin/api/me');
      state.user = res.user;
      showApp();
      switchTab(state.tab);
    } catch (_) {
      logout();
    }
  }

  // ---------------- 2-Step Admin Login & OTP Box Helpers ----------------
  function initOTPBoxes() {
    const boxes = Array.from(document.querySelectorAll('#otp-boxes .otp-box'));
    boxes.forEach((box, idx) => {
      box.addEventListener('input', () => {
        box.value = box.value.replace(/\D/g, '').slice(-1);
        if (box.value && idx < boxes.length - 1) {
          boxes[idx + 1].focus();
        }
      });
      box.addEventListener('keydown', (e) => {
        if (e.key === 'Backspace' && !box.value && idx > 0) {
          boxes[idx - 1].focus();
        }
      });
      box.addEventListener('paste', (e) => {
        e.preventDefault();
        const digits = (e.clipboardData || window.clipboardData).getData('text').replace(/\D/g, '').slice(0, 6);
        digits.split('').forEach((d, offset) => {
          if (boxes[offset]) boxes[offset].value = d;
        });
        const targetIdx = Math.min(digits.length, boxes.length - 1);
        boxes[targetIdx].focus();
      });
    });
  }

  function readOTPBoxesValue() {
    return Array.from(document.querySelectorAll('#otp-boxes .otp-box')).map((b) => b.value).join('');
  }

  function startResendCooldown() {
    const btn = $('btn-resend-email');
    if (state.resendTimer) clearInterval(state.resendTimer);
    let secs = 45;
    btn.disabled = true;
    btn.textContent = `重新发送 (${secs}s)`;
    state.resendTimer = setInterval(() => {
      secs -= 1;
      if (secs <= 0) {
        clearInterval(state.resendTimer);
        state.resendTimer = null;
        btn.disabled = false;
        btn.textContent = '重新发送邮箱验证码';
      } else {
        btn.textContent = `重新发送 (${secs}s)`;
      }
    }, 1000);
  }

  function enterStep2Challenge(mode, res) {
    state.pendingLogin.mode = mode;
    state.pendingLogin.useRecovery = false;
    $('login-step-1').classList.add('hidden');
    $('login-step-2').classList.remove('hidden');
    $('login-otp-error').classList.add('hidden');
    $('otp-boxes-wrap').classList.remove('hidden');
    $('recovery-input-wrap').classList.add('hidden');

    if (mode === 'tfa_check') {
      $('otp-title').textContent = '动态口令两步验证 (2FA)';
      $('otp-desc').textContent = '请打开验证器 App 输入 6 位动态验证码';
      $('btn-toggle-recovery').classList.remove('hidden');
      $('btn-toggle-recovery').textContent = '改用备用恢复码';
      $('btn-resend-email').classList.add('hidden');
    } else {
      $('otp-title').textContent = '邮箱登录安全验证';
      $('otp-desc').textContent = `验证码已发送至 ${res.masked_email || '管理员邮箱'}，10 分钟内有效`;
      $('btn-toggle-recovery').classList.add('hidden');
      $('btn-resend-email').classList.remove('hidden');
      startResendCooldown();
    }
    const firstBox = document.querySelector('#otp-boxes .otp-box');
    if (firstBox) firstBox.focus();
  }

  initOTPBoxes();

  $('btn-theme').addEventListener('click', () => {
    const cur = document.documentElement.getAttribute('data-theme');
    applyTheme(cur === 'dark' ? 'light' : 'dark');
  });

  $('btn-menu').addEventListener('click', openSidebar);
  $('sidebar-backdrop').addEventListener('click', closeSidebar);

  $('btn-toggle-pw').addEventListener('click', () => {
    const input = $('login-password');
    input.type = input.type === 'password' ? 'text' : 'password';
  });

  $('login-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const errEl = $('login-error');
    const submitBtn = $('btn-login-submit');
    errEl.classList.add('hidden');
    submitBtn.disabled = true;
    submitBtn.textContent = '正在验证...';
    const username = $('login-username').value.trim();
    const password = $('login-password').value;
    try {
      const res = await api('/admin/api/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      });
      if (res.type === 'tfa_check' || res.type === 'email_check') {
        state.pendingLogin = { username, password, mode: res.type, useRecovery: false };
        enterStep2Challenge(res.type, res);
        return;
      }
      state.token = res.access_token;
      state.user = res.user;
      localStorage.setItem('rd_admin_token', state.token);
      $('login-password').value = '';
      showApp();
      switchTab('dashboard');
    } catch (err) {
      errEl.textContent = err.message;
      errEl.classList.remove('hidden');
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = '登录控制台';
    }
  });

  $('btn-toggle-recovery').addEventListener('click', () => {
    if (!state.pendingLogin) return;
    state.pendingLogin.useRecovery = !state.pendingLogin.useRecovery;
    const useRec = state.pendingLogin.useRecovery;
    $('otp-boxes-wrap').classList.toggle('hidden', useRec);
    $('recovery-input-wrap').classList.toggle('hidden', !useRec);
    $('btn-toggle-recovery').textContent = useRec ? '改用 6 位动态验证码' : '改用备用恢复码';
    if (useRec) $('login-recovery-input').focus();
  });

  $('btn-resend-email').addEventListener('click', async () => {
    if (!state.pendingLogin) return;
    $('login-otp-error').classList.add('hidden');
    try {
      await api('/admin/api/login', {
        method: 'POST',
        body: JSON.stringify({
          username: state.pendingLogin.username,
          password: state.pendingLogin.password,
        }),
      });
      toast('已重新发送邮箱验证码');
      startResendCooldown();
    } catch (err) {
      $('login-otp-error').textContent = err.message;
      $('login-otp-error').classList.remove('hidden');
    }
  });

  $('btn-otp-back').addEventListener('click', resetLoginSteps);

  $('login-otp-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    if (!state.pendingLogin) return;
    const errEl = $('login-otp-error');
    const submitBtn = $('btn-otp-submit');
    errEl.classList.add('hidden');

    const code = state.pendingLogin.useRecovery
      ? $('login-recovery-input').value.trim()
      : readOTPBoxesValue();
    if (!code || (!state.pendingLogin.useRecovery && code.length < 6)) {
      errEl.textContent = state.pendingLogin.useRecovery ? '请输入备用恢复码' : '请输入完整的 6 位数字验证码';
      errEl.classList.remove('hidden');
      return;
    }

    const body = {
      username: state.pendingLogin.username,
      password: state.pendingLogin.password,
    };
    if (state.pendingLogin.mode === 'tfa_check') {
      body.tfa_code = code;
    } else {
      body.verification_code = code;
    }

    submitBtn.disabled = true;
    submitBtn.textContent = '验证中...';
    try {
      const res = await api('/admin/api/login', { method: 'POST', body: JSON.stringify(body) });
      state.token = res.access_token;
      state.user = res.user;
      localStorage.setItem('rd_admin_token', state.token);
      $('login-password').value = '';
      showApp();
      switchTab('dashboard');
    } catch (err) {
      errEl.textContent = err.message;
      errEl.classList.remove('hidden');
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = '验证并登录';
    }
  });

  $('btn-logout').addEventListener('click', logout);
  $('btn-go-security').addEventListener('click', () => {
    closeSidebar();
    switchTab('security');
  });
  $('modal-close').addEventListener('click', closeModal);
  $('modal-backdrop').addEventListener('click', (e) => {
    if (e.target === $('modal-backdrop')) closeModal();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closeModal();
      closeSidebar();
    }
  });

  $('btn-change-pw').addEventListener('click', () => {
    openModal(
      '修改管理员密码',
      `<form id="form-pw" class="stack">
        <label class="field"><span>当前密码</span><input name="old_password" type="password" required></label>
        <label class="field"><span>新密码（至少 6 位）</span><input name="new_password" type="password" minlength="6" required></label>
        <button class="btn btn-primary" type="submit">保存新密码</button>
      </form>`,
      (root) => {
        root.querySelector('#form-pw').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          try {
            await api('/admin/api/me/password', {
              method: 'PUT',
              body: JSON.stringify(Object.fromEntries(fd.entries())),
            });
            closeModal();
            toast('密码修改成功');
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  });

  $('sidebar-nav').addEventListener('click', (e) => {
    const btn = e.target.closest('.nav-item');
    if (!btn) return;
    closeSidebar();
    switchTab(btn.dataset.tab);
  });

  const tabMeta = {
    dashboard: { title: '概览仪表盘', subtitle: '实时运行状态、手机扫码配置与最近活动' },
    devices: { title: '设备与权限组', subtitle: '查看受管客户端在线状态、硬件信息与设备组访问授权' },
    ab: { title: '地址簿与标签', subtitle: '管理个人与共享地址簿、标签颜色及成员共享规则' },
    users: { title: '用户与分组', subtitle: '管理本地与 SSO/LDAP 用户、分组权限与二次验证状态' },
    security: { title: '安全与验证 (2FA)', subtitle: '管理 TOTP 两步验证、SMTP 邮件服务器与登录验证策略' },
    sso: { title: '第三方登录 (SSO / LDAP)', subtitle: '配置 OIDC、GitHub、Google 单点登录与 LDAP/AD 目录集成' },
    audit: { title: '审计与登录日志', subtitle: '追溯远程会话连接、文件传输、告警事件与登录历史' },
    system: { title: '配置与官方更新', subtitle: '获取二维码/一键导入配置串并检查官方 RustDesk 最新版本' },
  };

  function switchTab(tab) {
    state.tab = tab;
    document.querySelectorAll('.nav-item').forEach((b) => {
      b.classList.toggle('active', b.dataset.tab === tab);
    });
    const meta = tabMeta[tab] || { title: tab, subtitle: '' };
    $('page-title').textContent = meta.title;
    $('page-subtitle').textContent = meta.subtitle;
    $('topbar-actions').innerHTML = '';
    const renderers = {
      dashboard: renderDashboard,
      devices: renderDevices,
      ab: renderAddressBooks,
      users: renderUsers,
      security: renderSecurity,
      sso: renderSSO,
      audit: renderAudit,
      system: renderSystem,
    };
    (renderers[tab] || renderDashboard)();
  }

  // ---------------- 1. Dashboard ----------------
  async function renderDashboard() {
    $('topbar-actions').innerHTML = `<button class="btn btn-secondary btn-sm" id="btn-refresh-dash">刷新数据</button>`;
    $('btn-refresh-dash').onclick = renderDashboard;
    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const res = await api('/admin/api/dashboard');
      const s = res.stats || {};
      const cfg = res.server_config || {};
      const totalPeers = Number(s.peers || 0);
      const onlinePeers = Number(s.online_peers || 0);
      const onlineRate = totalPeers > 0 ? Math.round((onlinePeers / totalPeers) * 100) : 0;

      area.innerHTML = `
        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-head">
              <span class="stat-label">系统用户</span>
              <span class="stat-icon blue"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/></svg></span>
            </div>
            <div class="stat-value">${esc(s.users)}</div>
            <div class="stat-foot">含本地、LDAP 与 SSO 账号</div>
          </div>

          <div class="stat-card">
            <div class="stat-head">
              <span class="stat-label">在线 / 受管设备</span>
              <span class="stat-icon green"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8"/><path d="M12 17v4"/></svg></span>
            </div>
            <div class="stat-value">${esc(onlinePeers)} <span style="font-size:15px;color:var(--muted);font-weight:500;">/ ${esc(totalPeers)}</span></div>
            <div class="progress-track"><div class="progress-fill" style="width:${onlineRate}%"></div></div>
            <div class="stat-foot">当前在线率 ${onlineRate}%</div>
          </div>

          <div class="stat-card">
            <div class="stat-head">
              <span class="stat-label">地址簿集合</span>
              <span class="stat-icon blue"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20"/></svg></span>
            </div>
            <div class="stat-value">${esc(s.address_books)}</div>
            <div class="stat-foot">个人 + 团队共享地址簿</div>
          </div>

          <div class="stat-card">
            <div class="stat-head">
              <span class="stat-label">24h 连接审计</span>
              <span class="stat-icon amber"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg></span>
            </div>
            <div class="stat-value">${esc(s.conns_24h)}</div>
            <div class="stat-foot">近 24 小时会话事件</div>
          </div>

          <div class="stat-card">
            <div class="stat-head">
              <span class="stat-label">24h 登录次数</span>
              <span class="stat-icon green"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"/></svg></span>
            </div>
            <div class="stat-value">${esc(s.logins_24h)}</div>
            <div class="stat-foot">客户端与管理台认证统计</div>
          </div>
        </div>

        <div class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">官方服务端连接参数 & 移动端扫码配置</h3>
              <div class="cell-sub">公钥来源: ${esc(cfg.key_source)} · API 版本 v${esc(cfg.app_version || '1.0.0')}</div>
            </div>
            <div class="row">
              <span class="badge ${cfg.key_exists ? 'badge-green' : 'badge-red'}">${cfg.key_exists ? '已读取官方公钥' : '未找到公钥'}</span>
              <button class="btn btn-primary btn-sm" id="copy-cfg-str">复制一键配置串</button>
              <button class="btn btn-secondary btn-sm" id="copy-cli-cmd">复制 CLI 命令</button>
            </div>
          </div>
          <div class="grid-2" style="align-items:center;">
            <div class="stack">
              <div class="kv-grid">
                <div class="kv-item"><span class="kv-key">ID 服务器</span><span class="kv-val">${esc(cfg.id_server || '-')}</span><button class="btn btn-xs btn-secondary" data-copy-val="${esc(cfg.id_server)}">复制</button></div>
                <div class="kv-item"><span class="kv-key">中继服务器</span><span class="kv-val">${esc(cfg.relay_server || '-')}</span><button class="btn btn-xs btn-secondary" data-copy-val="${esc(cfg.relay_server)}">复制</button></div>
                <div class="kv-item"><span class="kv-key">API 服务器</span><span class="kv-val">${esc(cfg.api_server || '-')}</span><button class="btn btn-xs btn-secondary" data-copy-val="${esc(cfg.api_server)}">复制</button></div>
                <div class="kv-item"><span class="kv-key">Official Key</span><span class="kv-val">${esc(cfg.public_key || '等待官方 hbbs 生成 id_ed25519.pub')}</span><button class="btn btn-xs btn-secondary" id="copy-pub-key">复制</button></div>
              </div>
              <div class="code-box" title="桌面/移动端导入配置串">${esc(cfg.config_string || '-')}</div>
            </div>

            <div class="qr-panel">
              ${cfg.qr_data_uri ? `<div class="qr-frame"><img src="${esc(cfg.qr_data_uri)}" alt="RustDesk 配置二维码"></div>` : ''}
              <div>
                <strong style="font-size:13.5px;">RustDesk 手机端扫码一键导入</strong>
                <div class="cell-sub">打开 RustDesk 手机客户端「网络设置」→ 点击右上角扫码即可自动填入全部参数</div>
              </div>
            </div>
          </div>
        </div>

        <div class="grid-2">
          <div class="card">
            <div class="card-header"><h3 class="card-title">最近连接审计</h3></div>
            <div class="table-wrap">
              ${(res.recent_conns || []).length ? `
                <table>
                  <thead><tr><th>动作</th><th>被控 ID</th><th>来源</th><th>IP</th><th>时间</th></tr></thead>
                  <tbody>
                    ${res.recent_conns.map((r) => `
                      <tr>
                        <td><span class="badge badge-blue">${esc(r.action || '-')}</span></td>
                        <td><button class="copy-pill" data-copy-val="${esc(r.peer_id)}">${esc(r.peer_id)}</button></td>
                        <td>${esc(r.from_peer)} ${r.from_name ? `<span class="cell-sub">(${esc(r.from_name)})</span>` : ''}</td>
                        <td>${esc(r.ip)}</td>
                        <td title="${esc(fmtTime(r.created_at))}">${esc(relTime(r.created_at))}</td>
                      </tr>`).join('')}
                  </tbody>
                </table>` : renderEmpty('暂无连接记录', '客户端发起远程连接后会自动记录到此')}
            </div>
          </div>

          <div class="card">
            <div class="card-header"><h3 class="card-title">最近登录日志</h3></div>
            <div class="table-wrap">
              ${(res.recent_logins || []).length ? `
                <table>
                  <thead><tr><th>用户</th><th>认证方式</th><th>状态</th><th>IP</th><th>时间</th></tr></thead>
                  <tbody>
                    ${res.recent_logins.map((r) => `
                      <tr>
                        <td><strong>${esc(r.username)}</strong></td>
                        <td><span class="badge badge-gray">${esc(r.auth_source)}</span></td>
                        <td><span class="badge ${r.status === 'success' ? 'badge-green' : 'badge-red'}">${esc(r.status)}</span></td>
                        <td>${esc(r.ip)}</td>
                        <td title="${esc(fmtTime(r.created_at))}">${esc(relTime(r.created_at))}</td>
                      </tr>`).join('')}
                  </tbody>
                </table>` : renderEmpty('暂无登录记录', '用户登录客户端或管理后台后显示')}
            </div>
          </div>
        </div>
      `;

      $('copy-cfg-str').onclick = () => copyText(cfg.config_string, '已复制一键导入配置串');
      $('copy-cli-cmd').onclick = () => copyText(cfg.cli_command, '已复制 CLI 部署命令');
      $('copy-pub-key').onclick = () => copyText(cfg.public_key, '已复制官方 Public Key');
      area.querySelectorAll('[data-copy-val]').forEach((btn) => {
        btn.onclick = () => copyText(btn.dataset.copyVal);
      });
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  // ---------------- 2. Users & Groups ----------------
  async function renderUsers() {
    $('topbar-actions').innerHTML = `
      <button class="btn btn-secondary btn-sm" id="btn-add-group">+ 新建用户组</button>
      <button class="btn btn-primary btn-sm" id="btn-add-user">+ 新建用户</button>
    `;
    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const pageSize = 20;
      const fetchSize = state.usersStatusFilter === 'all' ? pageSize : 200;
      const fetchPage = state.usersStatusFilter === 'all' ? state.usersPage : 1;

      const [uRes, gRes] = await Promise.all([
        api(`/admin/api/users?current=${fetchPage}&pageSize=${fetchSize}&keyword=${encodeURIComponent(state.usersKeyword)}`),
        api('/admin/api/groups'),
      ]);
      const allUsers = uRes.data || [];
      const groups = gRes.data || [];

      let visibleUsers = allUsers;
      let totalCount = Number(uRes.total || 0);
      if (state.usersStatusFilter !== 'all') {
        const want = state.usersStatusFilter === 'enabled' ? 1 : 0;
        const filtered = allUsers.filter((u) => u.status === want);
        totalCount = filtered.length;
        const start = (state.usersPage - 1) * pageSize;
        visibleUsers = filtered.slice(start, start + pageSize);
      }

      area.innerHTML = `
        <div class="tabs">
          <button class="tab-btn ${state.usersSubTab === 'users' ? 'active' : ''}" data-utab="users">用户列表 (${esc(uRes.total)})</button>
          <button class="tab-btn ${state.usersSubTab === 'groups' ? 'active' : ''}" data-utab="groups">用户分组 (${groups.length})</button>
        </div>

        ${state.usersSubTab === 'users' ? `
          <div class="card">
            <div class="filter-bar">
              <div class="row">
                <input id="user-filter" class="search-input" value="${esc(state.usersKeyword)}" placeholder="搜索用户名 / 邮箱 / 备注（回车）...">
                <button id="btn-user-search" class="btn btn-secondary btn-sm">搜索</button>
              </div>
              <div class="pill-group">
                <button class="pill-btn ${state.usersStatusFilter === 'all' ? 'active' : ''}" data-ustatus="all">全部</button>
                <button class="pill-btn ${state.usersStatusFilter === 'enabled' ? 'active' : ''}" data-ustatus="enabled">启用</button>
                <button class="pill-btn ${state.usersStatusFilter === 'disabled' ? 'active' : ''}" data-ustatus="disabled">禁用</button>
              </div>
            </div>

            <div class="table-wrap">
              ${visibleUsers.length ? `
                <table>
                  <thead><tr><th>用户</th><th>分组 / 来源</th><th>安全验证</th><th>角色</th><th>状态</th><th>最后登录</th><th>操作</th></tr></thead>
                  <tbody>
                    ${visibleUsers.map((u) => `
                      <tr>
                        <td>
                          <div class="user-cell">
                            <span class="user-avatar" style="background:${avatarColor(u.username)}">${esc((u.username || 'U').charAt(0).toUpperCase())}</span>
                            <div>
                              <strong>${esc(u.username)}</strong>
                              <div class="cell-sub">${esc(u.email || u.note || '无邮箱')}</div>
                            </div>
                          </div>
                        </td>
                        <td>
                          <div><span class="badge badge-gray">${esc(u.group_name || 'default')}</span></div>
                          <div class="cell-sub">${esc(u.auth_source)}</div>
                        </td>
                        <td>
                          <div class="row" style="gap:4px;">
                            ${u.totp_enabled ? '<span class="badge badge-green">2FA 已开启</span>' : '<span class="badge badge-gray">2FA 未开启</span>'}
                            ${u.email_verify_enabled ? '<span class="badge badge-blue">邮箱必验</span>' : ''}
                          </div>
                        </td>
                        <td>${u.is_admin ? '<span class="badge badge-amber">管理员</span>' : '普通用户'}</td>
                        <td>${u.status === 1 ? '<span class="badge badge-green">启用</span>' : '<span class="badge badge-red">禁用</span>'}</td>
                        <td title="${esc(fmtTime(u.last_login_at))}">${esc(relTime(u.last_login_at))}</td>
                        <td>
                          <div class="row">
                            <button class="btn btn-xs btn-secondary" data-edit-user="${u.id}">编辑</button>
                            <button class="btn btn-xs btn-secondary" data-revoke-user="${u.id}">踢下线</button>
                            ${u.totp_enabled ? `<button class="btn btn-xs btn-secondary" data-reset-2fa="${u.id}">重置 2FA</button>` : ''}
                            ${u.id !== state.user.id ? `<button class="btn btn-xs btn-danger" data-del-user="${u.id}">删除</button>` : ''}
                          </div>
                        </td>
                      </tr>`).join('')}
                  </tbody>
                </table>` : renderEmpty('没有匹配的用户', '可调整搜索关键词或点击右上角新建用户')}
            </div>
            ${renderPagination(state.usersPage, pageSize, totalCount)}
          </div>
        ` : `
          <div class="card">
            <div class="card-header"><h3 class="card-title">用户分组列表</h3></div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>ID</th><th>组名</th><th>备注</th><th>操作</th></tr></thead>
                <tbody>
                  ${groups.map((g) => `
                    <tr>
                      <td>${esc(g.id)}</td>
                      <td><strong>${esc(g.name)}</strong></td>
                      <td>${esc(g.note || '-')}</td>
                      <td>
                        <div class="row">
                          <button class="btn btn-xs btn-secondary" data-edit-grp="${g.id}">编辑</button>
                          ${g.name !== 'default' ? `<button class="btn btn-xs btn-danger" data-del-grp="${g.id}">删除</button>` : ''}
                        </div>
                      </td>
                    </tr>`).join('')}
                </tbody>
              </table>
            </div>
          </div>
        `}
      `;

      $('btn-add-user').onclick = () => openUserModal(null, groups);
      $('btn-add-group').onclick = () => openGroupModal(null);

      area.querySelectorAll('[data-utab]').forEach((b) => {
        b.onclick = () => {
          state.usersSubTab = b.dataset.utab;
          renderUsers();
        };
      });
      area.querySelectorAll('[data-ustatus]').forEach((b) => {
        b.onclick = () => {
          state.usersStatusFilter = b.dataset.ustatus;
          state.usersPage = 1;
          renderUsers();
        };
      });

      const doSearch = () => {
        state.usersKeyword = ($('user-filter')?.value || '').trim();
        state.usersPage = 1;
        renderUsers();
      };
      if ($('btn-user-search')) $('btn-user-search').onclick = doSearch;
      if ($('user-filter')) {
        $('user-filter').addEventListener('keydown', (e) => {
          if (e.key === 'Enter') doSearch();
        });
      }

      area.onclick = async (e) => {
        const pageBtn = e.target.closest('[data-page]');
        if (pageBtn && !pageBtn.disabled) {
          state.usersPage = Number(pageBtn.dataset.page);
          renderUsers();
          return;
        }
        const editU = e.target.dataset.editUser;
        const delU = e.target.dataset.delUser;
        const revU = e.target.dataset.revokeUser;
        const reset2FA = e.target.dataset.reset2fa;
        const editG = e.target.dataset.editGrp;
        const delG = e.target.dataset.delGrp;

        if (editU) {
          const u = allUsers.find((x) => String(x.id) === editU);
          if (u) openUserModal(u, groups);
        } else if (revU) {
          const ok = await confirmDialog({
            title: '强制下线确认',
            message: '确定要撤销该用户的所有客户端登录 Token 吗？对方需要重新登录。',
            confirmText: '确认踢下线',
            danger: false,
          });
          if (ok) {
            await api('/admin/api/users/' + revU + '/revoke-tokens', { method: 'POST' });
            toast('已撤销该用户所有客户端 Token');
          }
        } else if (reset2FA) {
          const ok = await confirmDialog({
            title: '重置用户 2FA',
            message: '确定清空该用户的 TOTP 两步验证与信任设备记录吗？',
            confirmText: '确认重置',
          });
          if (ok) {
            await api('/admin/api/users/' + reset2FA + '/reset-2fa', { method: 'POST' });
            toast('已重置该用户的 2FA');
            renderUsers();
          }
        } else if (delU) {
          const ok = await confirmDialog({
            title: '删除用户',
            message: '确定删除该用户吗？此操作不可撤销。',
            confirmText: '确认删除',
          });
          if (ok) {
            await api('/admin/api/users/' + delU, { method: 'DELETE' });
            toast('用户已删除');
            renderUsers();
          }
        } else if (editG) {
          const g = groups.find((x) => String(x.id) === editG);
          if (g) openGroupModal(g);
        } else if (delG) {
          const ok = await confirmDialog({
            title: '删除用户组',
            message: '确定删除该用户组吗？组内成员将自动迁移回 default 组。',
            confirmText: '确认删除',
          });
          if (ok) {
            await api('/admin/api/groups/' + delG, { method: 'DELETE' });
            toast('用户组已删除');
            renderUsers();
          }
        }
      };
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  function openUserModal(user, groups) {
    const isEdit = Boolean(user);
    openModal(
      isEdit ? '编辑用户: ' + user.username : '新建用户',
      `<form id="form-user" class="stack">
        ${isEdit ? '' : `<label class="field"><span>用户名</span><input name="username" type="text" required></label>`}
        <label class="field"><span>邮箱（用于邮箱登录验证码）</span><input name="email" type="email" value="${esc(user?.email || '')}"></label>
        <label class="field"><span>${isEdit ? '重置密码（留空则不修改）' : '密码（至少 6 位）'}</span><input name="password" type="password" ${isEdit ? '' : 'required minlength="6"'}></label>
        <label class="field"><span>所属用户组</span>
          <select name="group_id">
            ${groups.map((g) => `<option value="${g.id}" ${user && user.group_id === g.id ? 'selected' : ''}>${esc(g.name)}</option>`).join('')}
          </select>
        </label>
        <label class="field"><span>备注</span><input name="note" type="text" value="${esc(user?.note || '')}"></label>
        <label class="checkbox-row"><input name="status_chk" type="checkbox" ${!user || user.status === 1 ? 'checked' : ''}> 启用账号</label>
        <label class="checkbox-row"><input name="email_verify_enabled" type="checkbox" ${user?.email_verify_enabled ? 'checked' : ''}> 强制每次登录验证邮箱（需开启 SMTP 且填写邮箱）</label>
        <label class="checkbox-row"><input name="is_admin" type="checkbox" ${user?.is_admin ? 'checked' : ''}> 授予管理员权限</label>
        <button class="btn btn-primary" type="submit">保存</button>
      </form>`,
      (root) => {
        root.querySelector('#form-user').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          const payload = {
            username: fd.get('username'),
            email: fd.get('email'),
            password: fd.get('password'),
            group_id: Number(fd.get('group_id')),
            note: fd.get('note'),
            status: fd.get('status_chk') ? 1 : 0,
            email_verify_enabled: Boolean(fd.get('email_verify_enabled')),
            is_admin: Boolean(fd.get('is_admin')),
          };
          try {
            if (isEdit) {
              await api('/admin/api/users/' + user.id, { method: 'PUT', body: JSON.stringify(payload) });
            } else {
              await api('/admin/api/users', { method: 'POST', body: JSON.stringify(payload) });
            }
            closeModal();
            toast('用户信息已保存');
            renderUsers();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  function openGroupModal(grp) {
    const isEdit = Boolean(grp);
    openModal(
      isEdit ? '编辑用户组' : '新建用户组',
      `<form id="form-grp" class="stack">
        <label class="field"><span>组名</span><input name="name" type="text" value="${esc(grp?.name || '')}" required></label>
        <label class="field"><span>备注</span><input name="note" type="text" value="${esc(grp?.note || '')}"></label>
        <button class="btn btn-primary" type="submit">保存</button>
      </form>`,
      (root) => {
        root.querySelector('#form-grp').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          const payload = Object.fromEntries(fd.entries());
          try {
            if (isEdit) {
              await api('/admin/api/groups/' + grp.id, { method: 'PUT', body: JSON.stringify(payload) });
            } else {
              await api('/admin/api/groups', { method: 'POST', body: JSON.stringify(payload) });
            }
            closeModal();
            toast('用户组已保存');
            renderUsers();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  // ---------------- 3. Security & 2FA / Email Verification ----------------
  async function renderSecurity() {
    $('topbar-actions').innerHTML = `<button class="btn btn-secondary btn-sm" id="btn-refresh-sec">刷新状态</button>`;
    $('btn-refresh-sec').onclick = renderSecurity;
    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const [tfaInfo, smtpCfg] = await Promise.all([
        api('/admin/api/me/2fa'),
        api('/admin/api/smtp'),
      ]);
      const policy = smtpCfg.email_login_policy || 'off';

      area.innerHTML = `
        <div class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">我的两步验证 (TOTP Authenticator)</h3>
              <div class="cell-sub">兼容 Google Authenticator、Microsoft Authenticator、1Password、Bitwarden 等验证器</div>
            </div>
            <span class="badge ${tfaInfo.totp_enabled ? 'badge-green' : 'badge-gray'}">${tfaInfo.totp_enabled ? '已开启保护' : '未开启'}</span>
          </div>

          <div class="security-hero ${tfaInfo.totp_enabled ? 'active' : ''}">
            <div class="stack" style="gap:4px;">
              <strong style="font-size:15px;">${tfaInfo.totp_enabled ? '账号已启用 TOTP 动态口令保护' : '建议开启 TOTP 两步验证保护管理员与客户端登录'}</strong>
              <span class="cell-sub">
                ${tfaInfo.totp_enabled
                  ? `登录管理后台和 RustDesk 客户端均需输入动态验证码 · 剩余可用备用恢复码：${esc(tfaInfo.recovery_codes_remaining)} 个`
                  : '绑定验证器后将自动生成 6 个一次性备用恢复码，防止手机丢失导致无法登录'}
              </span>
            </div>
            <div class="row">
              ${tfaInfo.totp_enabled
                ? `<button id="btn-disable-2fa" class="btn btn-danger">关闭两步验证</button>`
                : `<button id="btn-enable-2fa" class="btn btn-primary">立即开启 2FA</button>`}
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">SMTP 邮件服务 & 邮箱登录验证策略</h3>
              <div class="cell-sub">用于发送登录一次性邮箱验证码（若用户同时开启 TOTP，优先使用 TOTP）</div>
            </div>
            <span class="badge ${smtpCfg.enabled ? 'badge-green' : 'badge-gray'}">${smtpCfg.enabled ? 'SMTP 已启用' : 'SMTP 未启用'}</span>
          </div>

          <form id="form-smtp" class="stack">
            <label class="checkbox-row"><input name="enabled" type="checkbox" ${smtpCfg.enabled ? 'checked' : ''}> 启用 SMTP 邮件发送服务</label>

            <div class="field">
              <span>全局邮箱登录验证策略</span>
              <div class="policy-grid" id="policy-cards">
                <label class="policy-card ${policy === 'off' ? 'selected' : ''}">
                  <input type="radio" name="email_login_policy" value="off" ${policy === 'off' ? 'checked' : ''}>
                  <strong>关闭全局强制</strong>
                  <span>仅当单个用户开启「强制邮箱验证」时触发</span>
                </label>
                <label class="policy-card ${policy === 'new_device' ? 'selected' : ''}">
                  <input type="radio" name="email_login_policy" value="new_device" ${policy === 'new_device' ? 'checked' : ''}>
                  <strong>仅新设备首次登录验证 (推荐)</strong>
                  <span>未知设备 UUID 需验证邮箱，通过后自动加入信任设备</span>
                </label>
                <label class="policy-card ${policy === 'always' ? 'selected' : ''}">
                  <input type="radio" name="email_login_policy" value="always" ${policy === 'always' ? 'checked' : ''}>
                  <strong>每次登录均验证</strong>
                  <span>所有有邮箱的用户每次密码登录都需要邮件验证码</span>
                </label>
              </div>
            </div>

            <div class="grid-2">
              <label class="field"><span>SMTP 服务器地址</span><input name="host" type="text" placeholder="smtp.qq.com / smtp.gmail.com" value="${esc(smtpCfg.host || '')}"></label>
              <div class="row">
                <label class="field" style="flex:1;"><span>端口</span><input id="smtp-port-input" name="port" type="number" value="${esc(smtpCfg.port || 587)}"></label>
                <label class="field" style="flex:1;"><span>加密方式</span>
                  <select id="smtp-sec-select" name="security">
                    <option value="starttls" ${smtpCfg.security === 'starttls' ? 'selected' : ''}>STARTTLS (587)</option>
                    <option value="ssl" ${smtpCfg.security === 'ssl' ? 'selected' : ''}>SSL / TLS (465)</option>
                    <option value="none" ${smtpCfg.security === 'none' ? 'selected' : ''}>无加密 (25)</option>
                  </select>
                </label>
              </div>
              <label class="field"><span>SMTP 用户名</span><input name="username" type="text" placeholder="noreply@example.com" value="${esc(smtpCfg.username || '')}"></label>
              <label class="field"><span>SMTP 密码 / 授权码 ${smtpCfg.has_password ? '(已配置，留空保持不变)' : ''}</span><input name="password" type="password" value=""></label>
              <label class="field"><span>发件人邮箱</span><input name="from_address" type="email" placeholder="noreply@example.com" value="${esc(smtpCfg.from_address || '')}"></label>
              <div class="row">
                <label class="field" style="flex:2;"><span>发件人名称</span><input name="from_name" type="text" value="${esc(smtpCfg.from_name || 'RustDesk API')}"></label>
                <label class="field" style="flex:1;"><span>验证码有效期(分)</span><input name="code_expire_minutes" type="number" min="1" max="60" value="${esc(smtpCfg.code_expire_minutes || 10)}"></label>
              </div>
            </div>

            <label class="checkbox-row"><input name="skip_tls_verify" type="checkbox" ${smtpCfg.skip_tls_verify ? 'checked' : ''}> 跳过 SMTP TLS 证书校验（仅内网自建邮件服务器勾选）</label>

            <div class="row">
              <button type="submit" class="btn btn-primary">保存 SMTP 与策略配置</button>
            </div>
          </form>

          <hr style="border:none;border-top:1px solid var(--border);margin:20px 0;">

          <div class="stack">
            <strong style="font-size:14px;">发送测试邮件验证配置</strong>
            <div class="row">
              <input id="smtp-test-to" class="search-input" type="email" style="flex:1;max-width:340px;" value="${esc(tfaInfo.email || '')}" placeholder="输入接收测试邮件的邮箱地址...">
              <button id="btn-test-smtp" type="button" class="btn btn-secondary">发送测试邮件</button>
            </div>
            <div id="smtp-test-result" class="hidden"></div>
          </div>
        </div>
      `;

      // Policy card active toggle
      area.querySelectorAll('#policy-cards .policy-card').forEach((card) => {
        card.addEventListener('click', () => {
          area.querySelectorAll('#policy-cards .policy-card').forEach((c) => c.classList.remove('selected'));
          card.classList.add('selected');
        });
      });

      // Suggest standard port on security change
      $('smtp-sec-select').addEventListener('change', (e) => {
        const portInput = $('smtp-port-input');
        const map = { starttls: 587, ssl: 465, none: 25 };
        if ([587, 465, 25, 0].includes(Number(portInput.value))) {
          portInput.value = map[e.target.value] || 587;
        }
      });

      const readSMTPForm = () => {
        const fd = new FormData($('form-smtp'));
        return {
          enabled: Boolean(fd.get('enabled')),
          email_login_policy: fd.get('email_login_policy') || 'off',
          host: fd.get('host'),
          port: Number(fd.get('port')) || 587,
          security: fd.get('security'),
          username: fd.get('username'),
          password: fd.get('password'),
          from_address: fd.get('from_address'),
          from_name: fd.get('from_name'),
          code_expire_minutes: Number(fd.get('code_expire_minutes')) || 10,
          skip_tls_verify: Boolean(fd.get('skip_tls_verify')),
        };
      };

      $('form-smtp').onsubmit = async (ev) => {
        ev.preventDefault();
        try {
          await api('/admin/api/smtp', { method: 'PUT', body: JSON.stringify(readSMTPForm()) });
          toast('SMTP 与验证策略已保存');
          renderSecurity();
        } catch (err) {
          toast(err.message, false);
        }
      };

      $('btn-test-smtp').onclick = async () => {
        const to = $('smtp-test-to').value.trim();
        const box = $('smtp-test-result');
        if (!to) {
          toast('请输入测试收件邮箱', false);
          return;
        }
        box.className = 'info-banner';
        box.textContent = '正在连接 SMTP 服务器并发送测试邮件...';
        try {
          const payload = Object.assign(readSMTPForm(), { to });
          const res = await api('/admin/api/smtp/test', { method: 'POST', body: JSON.stringify(payload) });
          box.className = 'success-banner';
          box.textContent = res.message || '测试邮件发送成功！';
        } catch (err) {
          box.className = 'error-banner';
          box.textContent = '发送失败：' + err.message;
        }
      };

      if ($('btn-enable-2fa')) {
        $('btn-enable-2fa').onclick = openSetup2FAModal;
      }
      if ($('btn-disable-2fa')) {
        $('btn-disable-2fa').onclick = openDisable2FAModal;
      }
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  async function openSetup2FAModal() {
    openModal('绑定 TOTP 两步验证器', '<div class="skeleton-line"></div><div class="skeleton-line" style="width:70%"></div>');
    try {
      const setup = await api('/admin/api/me/2fa/setup', { method: 'POST' });
      openModal(
        '绑定 TOTP 两步验证器',
        `<div class="stack">
          <div class="qr-panel">
            <div class="qr-frame"><img src="${esc(setup.qr_data_uri)}" alt="2FA 二维码"></div>
            <div class="cell-sub">使用 Google Authenticator / Microsoft Authenticator 扫描上方二维码</div>
          </div>

          <div class="kv-item">
            <span class="kv-key">手动密钥</span>
            <span class="kv-val">${esc(setup.secret)}</span>
            <button type="button" id="btn-copy-2fa-sec" class="btn btn-xs btn-secondary">复制</button>
          </div>

          <form id="form-confirm-2fa" class="stack">
            <label class="field">
              <span>输入验证器显示的 6 位动态验证码以完成启用</span>
              <input name="code" type="text" inputmode="numeric" maxlength="6" placeholder="123456" required>
            </label>
            <button type="submit" class="btn btn-primary btn-block">验证并开启 2FA</button>
          </form>
        </div>`,
        (root) => {
          root.querySelector('#btn-copy-2fa-sec').onclick = () => copyText(setup.secret, '已复制 TOTP 密钥');
          root.querySelector('#form-confirm-2fa').addEventListener('submit', async (ev) => {
            ev.preventDefault();
            const code = new FormData(ev.target).get('code');
            try {
              const res = await api('/admin/api/me/2fa/confirm', {
                method: 'POST',
                body: JSON.stringify({ code }),
              });
              toast('2FA 已成功开启');
              showRecoveryCodesModal(res.recovery_codes || []);
            } catch (err) {
              toast(err.message, false);
            }
          });
        }
      );
    } catch (err) {
      closeModal();
      toast(err.message, false);
    }
  }

  function showRecoveryCodesModal(codes) {
    const joined = codes.join('\n');
    openModal(
      '保存您的备用恢复码',
      `<div class="stack">
        <div class="info-banner" style="margin:0;">
          <span>请妥善保管以下 6 个一次性备用恢复码！若更换或丢失手机，可用于应急登录。此窗口关闭后将无法再次查看完整明文。</span>
        </div>
        <div class="recovery-grid">
          ${codes.map((c) => `<div class="recovery-code">${esc(c)}</div>`).join('')}
        </div>
        <div class="row" style="justify-content:flex-end;">
          <button type="button" id="btn-dl-recovery" class="btn btn-secondary">下载 .txt 备份</button>
          <button type="button" id="btn-copy-recovery" class="btn btn-secondary">复制全部</button>
          <button type="button" id="btn-done-recovery" class="btn btn-primary">我已保存</button>
        </div>
      </div>`,
      (root) => {
        root.querySelector('#btn-copy-recovery').onclick = () => copyText(joined, '已复制全部恢复码');
        root.querySelector('#btn-dl-recovery').onclick = () => {
          const blob = new Blob(['RustDesk API 2FA 备用恢复码（每个仅限使用一次）:\n\n' + joined + '\n'], { type: 'text/plain;charset=utf-8' });
          const url = URL.createObjectURL(blob);
          const a = document.createElement('a');
          a.href = url;
          a.download = 'rustdesk-2fa-recovery-codes.txt';
          a.click();
          URL.revokeObjectURL(url);
        };
        root.querySelector('#btn-done-recovery').onclick = () => {
          closeModal();
          renderSecurity();
        };
      }
    );
  }

  function openDisable2FAModal() {
    openModal(
      '关闭两步验证 (2FA)',
      `<form id="form-disable-2fa" class="stack">
        <div class="error-banner">关闭后，登录账号将不再要求验证器动态口令。</div>
        <label class="field"><span>当前管理员密码</span><input name="password" type="password" required></label>
        <label class="field"><span>当前 6 位动态验证码（或备用恢复码）</span><input name="code" type="text" required></label>
        <button type="submit" class="btn btn-danger-solid btn-block">确认关闭 2FA</button>
      </form>`,
      (root) => {
        root.querySelector('#form-disable-2fa').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          try {
            await api('/admin/api/me/2fa/disable', {
              method: 'POST',
              body: JSON.stringify(Object.fromEntries(fd.entries())),
            });
            closeModal();
            toast('2FA 已关闭');
            renderSecurity();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  // ---------------- 4. Devices & Device Groups ----------------
  async function renderDevices() {
    $('topbar-actions').innerHTML = `<button class="btn btn-primary btn-sm" id="btn-add-dg">+ 新建设备分组</button>`;
    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const pageSize = 20;
      const fetchSize = state.devicesOnlineFilter === 'all' ? pageSize : 500;
      const fetchPage = state.devicesOnlineFilter === 'all' ? state.devicesPage : 1;
      const dgParam = state.devicesGroupFilter ? '&device_group_id=' + encodeURIComponent(state.devicesGroupFilter) : '';

      const [dRes, dgRes, uRes, gRes] = await Promise.all([
        api(`/admin/api/devices?current=${fetchPage}&pageSize=${fetchSize}&keyword=${encodeURIComponent(state.devicesKeyword)}${dgParam}`),
        api('/admin/api/device-groups'),
        api('/admin/api/users?pageSize=200'),
        api('/admin/api/groups'),
      ]);
      const rawDevices = dRes.data || [];
      const deviceGroups = dgRes.data || [];
      const users = uRes.data || [];
      const userGroups = gRes.data || [];

      let visibleDevices = rawDevices;
      let totalCount = Number(dRes.total || 0);
      if (state.devicesOnlineFilter !== 'all') {
        const wantOnline = state.devicesOnlineFilter === 'online';
        const filtered = rawDevices.filter((d) => Boolean(d.online) === wantOnline);
        totalCount = filtered.length;
        const start = (state.devicesPage - 1) * pageSize;
        visibleDevices = filtered.slice(start, start + pageSize);
      }

      area.innerHTML = `
        <div class="tabs">
          <button class="tab-btn ${state.devicesSubTab === 'devices' ? 'active' : ''}" data-dtab="devices">受管设备 (${esc(dRes.total)})</button>
          <button class="tab-btn ${state.devicesSubTab === 'groups' ? 'active' : ''}" data-dtab="groups">设备分组与 ACL (${deviceGroups.length})</button>
        </div>

        ${state.devicesSubTab === 'devices' ? `
          <div class="card">
            <div class="filter-bar">
              <div class="row">
                <input id="dev-filter" class="search-input" value="${esc(state.devicesKeyword)}" placeholder="搜索 ID / 主机名 / 别名 / 用户...">
                <select id="dev-dg-filter" class="select-input">
                  <option value="">全部设备组</option>
                  ${deviceGroups.map((dg) => `<option value="${dg.id}" ${String(state.devicesGroupFilter) === String(dg.id) ? 'selected' : ''}>${esc(dg.name)}</option>`).join('')}
                </select>
                <button id="btn-dev-search" class="btn btn-secondary btn-sm">筛选</button>
              </div>
              <div class="pill-group">
                <button class="pill-btn ${state.devicesOnlineFilter === 'all' ? 'active' : ''}" data-donline="all">全部</button>
                <button class="pill-btn ${state.devicesOnlineFilter === 'online' ? 'active' : ''}" data-donline="online">在线</button>
                <button class="pill-btn ${state.devicesOnlineFilter === 'offline' ? 'active' : ''}" data-donline="offline">离线</button>
              </div>
            </div>

            <div class="table-wrap">
              ${visibleDevices.length ? `
                <table>
                  <thead><tr><th>状态</th><th>RustDesk ID</th><th>设备名称 / 硬件信息</th><th>系统 / 客户端</th><th>归属用户 / 分组</th><th>最近活动</th><th>操作</th></tr></thead>
                  <tbody>
                    ${visibleDevices.map((d) => `
                      <tr>
                        <td>
                          <span class="badge ${d.online ? 'badge-green' : 'badge-gray'}">
                            <span class="status-dot ${d.online ? 'online' : 'offline'}"></span>
                            ${d.online ? '在线' : '离线'}
                          </span>
                        </td>
                        <td><button class="copy-pill" data-copy-val="${esc(d.peer_id)}" title="点击复制设备 ID">${esc(d.peer_id)}</button></td>
                        <td>
                          <strong>${esc(d.alias || d.hostname || '-')}</strong>
                          <div class="cell-sub">${esc(d.hostname || '')}${d.cpu ? ' · ' + esc(d.cpu) : ''}${d.memory ? ' · ' + esc(d.memory) + 'G' : ''}</div>
                        </td>
                        <td>
                          ${osBadge(d.os)}
                          <div class="cell-sub">${d.version ? 'v' + esc(d.version) : '-'}</div>
                        </td>
                        <td>
                          <div>${esc(d.user_name || '-')}</div>
                          <div class="cell-sub">${d.device_group_name ? `<span class="badge badge-blue">${esc(d.device_group_name)}</span>` : '未分组'}</div>
                        </td>
                        <td>
                          <div>${esc(d.last_ip || '-')}</div>
                          <div class="cell-sub" title="${esc(fmtTime(d.last_online_at))}">${esc(relTime(d.last_online_at))}</div>
                        </td>
                        <td>
                          <div class="row">
                            <button class="btn btn-xs btn-secondary" data-edit-dev="${d.id}">编辑</button>
                            <button class="btn btn-xs btn-danger" data-del-dev="${d.id}">删除</button>
                          </div>
                        </td>
                      </tr>`).join('')}
                  </tbody>
                </table>` : renderEmpty('暂无匹配设备', '客户端配置 API 地址后会自动通过心跳与 Sysinfo 上报注册')}
            </div>
            ${renderPagination(state.devicesPage, pageSize, totalCount)}
          </div>
        ` : `
          <div class="card">
            <div class="card-header"><h3 class="card-title">设备分组与访问控制 (ACL)</h3></div>
            <div class="table-wrap">
              ${deviceGroups.length ? `
                <table>
                  <thead><tr><th>ID</th><th>分组名称</th><th>授权访问的用户分组</th><th>备注</th><th>操作</th></tr></thead>
                  <tbody>
                    ${deviceGroups.map((dg) => `
                      <tr>
                        <td>${esc(dg.id)}</td>
                        <td><strong>${esc(dg.name)}</strong></td>
                        <td>${(dg.user_group_ids || []).map((id) => {
                          const ug = userGroups.find((x) => x.id === id);
                          return `<span class="badge badge-blue">${esc(ug ? ug.name : id)}</span>`;
                        }).join(' ') || '<span class="cell-sub">仅管理员可见</span>'}</td>
                        <td>${esc(dg.note || '-')}</td>
                        <td>
                          <div class="row">
                            <button class="btn btn-xs btn-secondary" data-edit-dg="${dg.id}">编辑 / 授权</button>
                            <button class="btn btn-xs btn-danger" data-del-dg="${dg.id}">删除</button>
                          </div>
                        </td>
                      </tr>`).join('')}
                  </tbody>
                </table>` : renderEmpty('暂无设备分组', '点击右上角创建设备分组并授权给指定用户组')}
            </div>
          </div>
        `}
      `;

      $('btn-add-dg').onclick = () => openDeviceGroupModal(null, userGroups);
      area.querySelectorAll('[data-dtab]').forEach((b) => {
        b.onclick = () => {
          state.devicesSubTab = b.dataset.dtab;
          renderDevices();
        };
      });
      area.querySelectorAll('[data-donline]').forEach((b) => {
        b.onclick = () => {
          state.devicesOnlineFilter = b.dataset.donline;
          state.devicesPage = 1;
          renderDevices();
        };
      });

      const applyDevFilter = () => {
        state.devicesKeyword = ($('dev-filter')?.value || '').trim();
        state.devicesGroupFilter = $('dev-dg-filter')?.value || '';
        state.devicesPage = 1;
        renderDevices();
      };
      if ($('btn-dev-search')) $('btn-dev-search').onclick = applyDevFilter;
      if ($('dev-dg-filter')) $('dev-dg-filter').onchange = applyDevFilter;
      if ($('dev-filter')) {
        $('dev-filter').addEventListener('keydown', (e) => {
          if (e.key === 'Enter') applyDevFilter();
        });
      }

      area.onclick = async (e) => {
        const copyBtn = e.target.closest('[data-copy-val]');
        if (copyBtn) {
          copyText(copyBtn.dataset.copyVal, '已复制设备 ID');
          return;
        }
        const pageBtn = e.target.closest('[data-page]');
        if (pageBtn && !pageBtn.disabled) {
          state.devicesPage = Number(pageBtn.dataset.page);
          renderDevices();
          return;
        }
        const editDev = e.target.dataset.editDev;
        const delDev = e.target.dataset.delDev;
        const editDG = e.target.dataset.editDg;
        const delDG = e.target.dataset.delDg;

        if (editDev) {
          const d = rawDevices.find((x) => String(x.id) === editDev);
          if (d) openDeviceModal(d, users, deviceGroups);
        } else if (delDev) {
          const ok = await confirmDialog({
            title: '删除设备记录',
            message: '确定删除该设备记录吗？设备下次上线时会重新上报注册。',
            confirmText: '确认删除',
          });
          if (ok) {
            await api('/admin/api/devices/' + delDev, { method: 'DELETE' });
            toast('设备记录已删除');
            renderDevices();
          }
        } else if (editDG) {
          const dg = deviceGroups.find((x) => String(x.id) === editDG);
          if (dg) openDeviceGroupModal(dg, userGroups);
        } else if (delDG) {
          const ok = await confirmDialog({
            title: '删除设备分组',
            message: '确定删除该设备分组吗？组内设备将变为未分组状态。',
            confirmText: '确认删除',
          });
          if (ok) {
            await api('/admin/api/device-groups/' + delDG, { method: 'DELETE' });
            toast('设备分组已删除');
            renderDevices();
          }
        }
      };
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  function openDeviceModal(dev, users, deviceGroups) {
    openModal(
      '编辑设备: ' + dev.peer_id,
      `<form id="form-dev" class="stack">
        <label class="field"><span>归属用户</span>
          <select name="user_id">
            <option value="0">-- 未绑定 --</option>
            ${users.map((u) => `<option value="${u.id}" ${dev.user_id === u.id ? 'selected' : ''}>${esc(u.username)}</option>`).join('')}
          </select>
        </label>
        <label class="field"><span>设备分组</span>
          <select name="device_group_id">
            <option value="0">-- 未分组 --</option>
            ${deviceGroups.map((dg) => `<option value="${dg.id}" ${dev.device_group_id === dg.id ? 'selected' : ''}>${esc(dg.name)}</option>`).join('')}
          </select>
        </label>
        <label class="field"><span>别名</span><input name="alias" type="text" value="${esc(dev.alias || '')}"></label>
        <label class="field"><span>备注</span><input name="note" type="text" value="${esc(dev.note || '')}"></label>
        <button class="btn btn-primary" type="submit">保存</button>
      </form>`,
      (root) => {
        root.querySelector('#form-dev').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          try {
            await api('/admin/api/devices/' + dev.id, {
              method: 'PUT',
              body: JSON.stringify({
                user_id: Number(fd.get('user_id')),
                device_group_id: Number(fd.get('device_group_id')),
                alias: fd.get('alias'),
                note: fd.get('note'),
              }),
            });
            closeModal();
            toast('设备信息已更新');
            renderDevices();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  function openDeviceGroupModal(dg, userGroups) {
    const isEdit = Boolean(dg);
    const checked = new Set(dg?.user_group_ids || []);
    openModal(
      isEdit ? '编辑设备分组' : '新建设备分组',
      `<form id="form-dg" class="stack">
        <label class="field"><span>分组名称</span><input name="name" type="text" value="${esc(dg?.name || '')}" required></label>
        <label class="field"><span>备注</span><input name="note" type="text" value="${esc(dg?.note || '')}"></label>
        <div class="field"><span>允许访问该设备组的用户分组</span>
          ${userGroups.map((ug) => `
            <label class="checkbox-row">
              <input type="checkbox" name="ug" value="${ug.id}" ${checked.has(ug.id) ? 'checked' : ''}> ${esc(ug.name)}
            </label>`).join('')}
        </div>
        <button class="btn btn-primary" type="submit">保存</button>
      </form>`,
      (root) => {
        root.querySelector('#form-dg').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          const ugIDs = fd.getAll('ug').map((x) => Number(x));
          const payload = { name: fd.get('name'), note: fd.get('note'), user_group_ids: ugIDs };
          try {
            if (isEdit) {
              await api('/admin/api/device-groups/' + dg.id, { method: 'PUT', body: JSON.stringify(payload) });
            } else {
              await api('/admin/api/device-groups', { method: 'POST', body: JSON.stringify(payload) });
            }
            closeModal();
            toast('设备分组已保存');
            renderDevices();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  // ---------------- 5. Address Books & Tags (Master-Detail) ----------------
  async function renderAddressBooks() {
    $('topbar-actions').innerHTML = `<button class="btn btn-primary btn-sm" id="btn-add-ab">+ 新建共享地址簿</button>`;
    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const [abRes, uRes, gRes] = await Promise.all([
        api('/admin/api/ab/collections'),
        api('/admin/api/users?pageSize=200'),
        api('/admin/api/groups'),
      ]);
      const cols = abRes.data || [];
      const users = uRes.data || [];
      const groups = gRes.data || [];
      if (!state.selectedABGuid || !cols.some((c) => c.guid === state.selectedABGuid)) {
        state.selectedABGuid = cols[0] ? cols[0].guid : '';
      }

      area.innerHTML = `
        <div class="split-layout">
          <div class="card">
            <div class="card-header">
              <h3 class="card-title">地址簿集合 (${cols.length})</h3>
            </div>
            <div class="collection-list">
              ${cols.map((c) => `
                <div class="collection-item ${c.guid === state.selectedABGuid ? 'active' : ''}" data-select-ab="${esc(c.guid)}">
                  <div class="collection-item-top">
                    <strong>${esc(c.name)}</strong>
                    ${c.is_personal ? '<span class="badge badge-gray">个人</span>' : '<span class="badge badge-blue">共享</span>'}
                  </div>
                  <div class="collection-item-meta">
                    <span>${esc(c.owner)} · ${esc(c.peer_count)} 台设备</span>
                    <div class="row" style="gap:4px;">
                      <button class="btn btn-xs btn-secondary" data-edit-ab="${esc(c.guid)}">改名</button>
                      ${!c.is_personal ? `<button class="btn btn-xs btn-danger" data-del-ab="${esc(c.guid)}">删除</button>` : ''}
                    </div>
                  </div>
                </div>`).join('') || renderEmpty('暂无地址簿', '点击右上角新建共享地址簿')}
            </div>
          </div>
          <div id="ab-detail-box"></div>
        </div>
      `;

      $('btn-add-ab').onclick = () => openCollectionModal(null);

      area.querySelector('.collection-list').onclick = async (e) => {
        const editBtn = e.target.closest('[data-edit-ab]');
        const delBtn = e.target.closest('[data-del-ab]');
        const item = e.target.closest('[data-select-ab]');
        if (editBtn) {
          e.stopPropagation();
          const c = cols.find((x) => x.guid === editBtn.dataset.editAb);
          if (c) openCollectionModal(c);
          return;
        }
        if (delBtn) {
          e.stopPropagation();
          const ok = await confirmDialog({
            title: '删除共享地址簿',
            message: '确定删除该共享地址簿及其包含的所有设备、标签和共享权限吗？',
            confirmText: '确认删除',
          });
          if (ok) {
            await api('/admin/api/ab/collections/' + encodeURIComponent(delBtn.dataset.delAb), { method: 'DELETE' });
            toast('共享地址簿已删除');
            renderAddressBooks();
          }
          return;
        }
        if (item) {
          state.selectedABGuid = item.dataset.selectAb;
          area.querySelectorAll('.collection-item').forEach((el) => {
            el.classList.toggle('active', el.dataset.selectAb === state.selectedABGuid);
          });
          loadABDetail(state.selectedABGuid, cols, users, groups);
        }
      };

      if (state.selectedABGuid) {
        loadABDetail(state.selectedABGuid, cols, users, groups);
      } else {
        $('ab-detail-box').innerHTML = `<div class="card">${renderEmpty('未选择地址簿', '请在左侧选择一个地址簿查看条目')}</div>`;
      }
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  function openCollectionModal(col) {
    const isEdit = Boolean(col);
    openModal(
      isEdit ? '编辑地址簿信息' : '新建共享地址簿',
      `<form id="form-ab" class="stack">
        <label class="field"><span>地址簿名称</span><input name="name" type="text" value="${esc(col?.name || '')}" required></label>
        <label class="field"><span>备注</span><input name="note" type="text" value="${esc(col?.note || '')}"></label>
        <button class="btn btn-primary" type="submit">${isEdit ? '保存' : '创建'}</button>
      </form>`,
      (root) => {
        root.querySelector('#form-ab').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          try {
            if (isEdit) {
              await api('/admin/api/ab/collections/' + encodeURIComponent(col.guid), {
                method: 'PUT',
                body: JSON.stringify(Object.fromEntries(fd.entries())),
              });
              toast('地址簿信息已更新');
            } else {
              const created = await api('/admin/api/ab/collections', {
                method: 'POST',
                body: JSON.stringify(Object.fromEntries(fd.entries())),
              });
              state.selectedABGuid = created.guid;
              toast('共享地址簿创建成功');
            }
            closeModal();
            renderAddressBooks();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  async function loadABDetail(guid, cols, users, groups) {
    const box = $('ab-detail-box');
    const col = cols.find((c) => c.guid === guid);
    if (!col) return;
    box.innerHTML = renderSkeleton(4);

    try {
      const [pRes, tRes, rRes] = await Promise.all([
        api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/peers'),
        api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/tags'),
        api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/rules'),
      ]);
      const peers = pRes.data || [];
      const tags = tRes.data || [];
      const rules = rRes.data || [];
      const ruleNames = { 1: '只读', 2: '读写', 3: '完全控制' };

      box.innerHTML = `
        <div class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">${esc(col.name)}</h3>
              <div class="cell-sub">GUID: <code>${esc(col.guid)}</code> · 拥有者: ${esc(col.owner)}</div>
            </div>
            <div class="row">
              <button class="btn btn-secondary btn-sm" id="btn-ab-add-tag">+ 添加标签</button>
              <button class="btn btn-primary btn-sm" id="btn-ab-add-peer">+ 添加设备</button>
              ${!col.is_personal ? `<button class="btn btn-secondary btn-sm" id="btn-ab-add-rule">+ 共享授权</button>` : ''}
            </div>
          </div>

          <div class="row" style="margin-bottom:14px;">
            <span class="stat-label" style="margin:0;">标签：</span>
            ${tags.map((t) => `
              <span class="tag-chip" style="background:${argbToHex(t.color)};">
                ${esc(t.name)}
                <button type="button" data-del-tag="${esc(t.name)}" title="删除标签">×</button>
              </span>`).join('') || '<span class="cell-sub">暂无标签</span>'}
          </div>

          <div class="filter-bar">
            <input id="ab-peer-search" class="search-input" value="${esc(state.abPeerKeyword)}" placeholder="筛选地址簿中的 ID / 别名 / 标签...">
            <span class="cell-sub">共 ${peers.length} 条目</span>
          </div>

          <div class="table-wrap" id="ab-peers-wrap"></div>

          ${!col.is_personal ? `
            <h4 style="margin:20px 0 10px;font-size:14px;">共享授权规则 (${rules.length})</h4>
            <div class="table-wrap">
              ${rules.length ? `
                <table>
                  <thead><tr><th>对象类型</th><th>授权对象</th><th>权限级别</th><th>操作</th></tr></thead>
                  <tbody>
                    ${rules.map((r) => {
                      const target = r.user_id
                        ? (users.find((u) => u.id === r.user_id)?.username || ('用户 #' + r.user_id))
                        : (groups.find((g) => g.id === r.group_id)?.name || ('用户组 #' + r.group_id));
                      return `<tr>
                        <td>${r.user_id ? '个人账号' : '用户分组'}</td>
                        <td><strong>${esc(target)}</strong></td>
                        <td><span class="badge badge-blue">${esc(ruleNames[r.rule] || r.rule)}</span></td>
                        <td><button class="btn btn-xs btn-danger" data-del-rule="${r.id}">撤销</button></td>
                      </tr>`;
                    }).join('')}
                  </tbody>
                </table>` : renderEmpty('尚未配置共享规则', '点击上方「+ 共享授权」可分享给其他用户或分组')}
            </div>` : ''}
        </div>
      `;

      const renderPeerRows = (kw = '') => {
        const l = kw.trim().toLowerCase();
        const filtered = peers.filter((p) =>
          !l ||
          (p.peer_id || '').toLowerCase().includes(l) ||
          (p.alias || '').toLowerCase().includes(l) ||
          (p.hostname || '').toLowerCase().includes(l) ||
          (p.tags || []).some((t) => t.toLowerCase().includes(l))
        );
        $('ab-peers-wrap').innerHTML = filtered.length ? `
          <table>
            <thead><tr><th>RustDesk ID</th><th>别名 / 主机名</th><th>平台</th><th>标签</th><th>操作</th></tr></thead>
            <tbody>
              ${filtered.map((p) => `
                <tr>
                  <td><button class="copy-pill" data-copy-val="${esc(p.peer_id)}">${esc(p.peer_id)}</button></td>
                  <td>
                    <strong>${esc(p.alias || p.hostname || '-')}</strong>
                    ${p.alias && p.hostname ? `<div class="cell-sub">${esc(p.hostname)}</div>` : ''}
                  </td>
                  <td>${osBadge(p.platform)}</td>
                  <td>${(p.tags || []).map((t) => `<span class="badge badge-gray">${esc(t)}</span>`).join(' ') || '-'}</td>
                  <td>
                    <div class="row">
                      <button class="btn btn-xs btn-secondary" data-edit-ab-peer="${esc(p.peer_id)}">编辑</button>
                      <button class="btn btn-xs btn-danger" data-del-ab-peer="${esc(p.peer_id)}">移除</button>
                    </div>
                  </td>
                </tr>`).join('')}
            </tbody>
          </table>` : renderEmpty('地址簿中暂无设备', '点击「+ 添加设备」或由客户端自动同步');
      };

      renderPeerRows(state.abPeerKeyword);
      $('ab-peer-search').addEventListener('input', (e) => {
        state.abPeerKeyword = e.target.value;
        renderPeerRows(state.abPeerKeyword);
      });

      const openPeerModal = (peerObj = null) => {
        const isEdit = Boolean(peerObj);
        openModal(
          isEdit ? '编辑地址簿设备' : '向地址簿添加设备',
          `<form id="form-ab-peer" class="stack">
            <label class="field"><span>RustDesk ID</span><input name="id" type="text" value="${esc(peerObj?.peer_id || '')}" ${isEdit ? 'readonly' : 'required'}></label>
            <label class="field"><span>别名</span><input name="alias" type="text" value="${esc(peerObj?.alias || '')}"></label>
            <label class="field"><span>主机名</span><input name="hostname" type="text" value="${esc(peerObj?.hostname || '')}"></label>
            <label class="field"><span>平台 (Windows / Linux / macOS)</span><input name="platform" type="text" value="${esc(peerObj?.platform || 'Windows')}"></label>
            <label class="field"><span>标签（逗号分隔）</span><input name="tags" type="text" value="${esc((peerObj?.tags || []).join(', '))}" placeholder="Ops,Server"></label>
            <label class="field"><span>备注</span><input name="note" type="text" value="${esc(peerObj?.note || '')}"></label>
            <button class="btn btn-primary" type="submit">保存</button>
          </form>`,
          (root) => {
            root.querySelector('#form-ab-peer').addEventListener('submit', async (ev) => {
              ev.preventDefault();
              const fd = new FormData(ev.target);
              const tagsArr = String(fd.get('tags') || '').split(',').map((s) => s.trim()).filter(Boolean);
              await api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/peers', {
                method: 'POST',
                body: JSON.stringify({
                  id: fd.get('id'),
                  alias: fd.get('alias'),
                  hostname: fd.get('hostname'),
                  platform: fd.get('platform'),
                  tags: tagsArr,
                  note: fd.get('note'),
                }),
              });
              closeModal();
              toast(isEdit ? '设备条目已更新' : '设备已添加到地址簿');
              loadABDetail(guid, cols, users, groups);
            });
          }
        );
      };

      $('btn-ab-add-peer').onclick = () => openPeerModal(null);

      $('btn-ab-add-tag').onclick = () => {
        openModal(
          '添加标签',
          `<form id="form-ab-tag" class="stack">
            <label class="field"><span>标签名称</span><input name="name" type="text" required></label>
            <label class="field"><span>标签颜色</span><input name="color" type="color" value="#2563eb"></label>
            <button class="btn btn-primary" type="submit">保存</button>
          </form>`,
          (root) => {
            root.querySelector('#form-ab-tag').addEventListener('submit', async (ev) => {
              ev.preventDefault();
              const fd = new FormData(ev.target);
              await api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/tags', {
                method: 'POST',
                body: JSON.stringify(Object.fromEntries(fd.entries())),
              });
              closeModal();
              toast('标签已保存');
              loadABDetail(guid, cols, users, groups);
            });
          }
        );
      };

      const ruleBtn = $('btn-ab-add-rule');
      if (ruleBtn) {
        ruleBtn.onclick = () => {
          openModal(
            '添加共享授权规则',
            `<form id="form-ab-rule" class="stack">
              <label class="field"><span>授权用户组（与指定用户二选一）</span>
                <select name="group_id">
                  <option value="0">-- 不选 --</option>
                  ${groups.map((g) => `<option value="${g.id}">${esc(g.name)}</option>`).join('')}
                </select>
              </label>
              <label class="field"><span>或指定单个用户</span>
                <select name="user_id">
                  <option value="0">-- 不选 --</option>
                  ${users.map((u) => `<option value="${u.id}">${esc(u.username)}</option>`).join('')}
                </select>
              </label>
              <label class="field"><span>权限级别</span>
                <select name="rule">
                  <option value="1">1 - 只读</option>
                  <option value="2">2 - 读写</option>
                  <option value="3">3 - 完全控制</option>
                </select>
              </label>
              <button class="btn btn-primary" type="submit">授予权限</button>
            </form>`,
            (root) => {
              root.querySelector('#form-ab-rule').addEventListener('submit', async (ev) => {
                ev.preventDefault();
                const fd = new FormData(ev.target);
                try {
                  await api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/rules', {
                    method: 'POST',
                    body: JSON.stringify({
                      group_id: Number(fd.get('group_id')),
                      user_id: Number(fd.get('user_id')),
                      rule: Number(fd.get('rule')),
                    }),
                  });
                  closeModal();
                  toast('共享规则已添加');
                  loadABDetail(guid, cols, users, groups);
                } catch (err) {
                  toast(err.message, false);
                }
              });
            }
          );
        };
      }

      box.onclick = async (e) => {
        const copyBtn = e.target.closest('[data-copy-val]');
        if (copyBtn) {
          copyText(copyBtn.dataset.copyVal, '已复制设备 ID');
          return;
        }
        const editPeer = e.target.dataset.editAbPeer;
        const delPeer = e.target.dataset.delAbPeer;
        const delTag = e.target.dataset.delTag;
        const delRule = e.target.dataset.delRule;

        if (editPeer) {
          const p = peers.find((x) => x.peer_id === editPeer);
          if (p) openPeerModal(p);
        } else if (delPeer) {
          const ok = await confirmDialog({
            title: '移除设备条目',
            message: `确定从当前地址簿移除设备 ${delPeer} 吗？`,
            confirmText: '确认移除',
          });
          if (ok) {
            await api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/peers/' + encodeURIComponent(delPeer), { method: 'DELETE' });
            toast('已移除');
            loadABDetail(guid, cols, users, groups);
          }
        } else if (delTag) {
          await api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/tags/' + encodeURIComponent(delTag), { method: 'DELETE' });
          toast('标签已删除');
          loadABDetail(guid, cols, users, groups);
        } else if (delRule) {
          await api('/admin/api/ab/collections/' + encodeURIComponent(guid) + '/rules/' + delRule, { method: 'DELETE' });
          toast('已撤销共享规则');
          loadABDetail(guid, cols, users, groups);
        }
      };
    } catch (err) {
      box.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  // ---------------- 6. Audit & Login Logs ----------------
  async function renderAudit() {
    $('topbar-actions').innerHTML = `<button class="btn btn-danger btn-sm" id="btn-purge-logs">清理历史日志</button>`;
    $('btn-purge-logs').onclick = () => {
      openModal(
        '清理历史审计日志',
        `<form id="form-purge" class="stack">
          <label class="field"><span>保留天数范围</span>
            <select name="days">
              <option value="30">清理 30 天之前的日志</option>
              <option value="90">清理 90 天之前的日志</option>
              <option value="7">清理 7 天之前的日志</option>
              <option value="0">清空全部历史日志</option>
            </select>
          </label>
          <button class="btn btn-danger-solid" type="submit">执行清理</button>
        </form>`,
        (root) => {
          root.querySelector('#form-purge').addEventListener('submit', async (ev) => {
            ev.preventDefault();
            const days = new FormData(ev.target).get('days');
            await api('/admin/api/audit/purge?days=' + encodeURIComponent(days), { method: 'DELETE' });
            closeModal();
            toast('历史日志清理完成');
            state.auditPage = 1;
            renderAudit();
          });
        }
      );
    };

    const area = $('content-area');
    const sub = state.auditSubTab;
    const pageSize = 30;
    area.innerHTML = `
      <div class="tabs">
        <button class="tab-btn ${sub === 'conn' ? 'active' : ''}" data-sub="conn">连接审计</button>
        <button class="tab-btn ${sub === 'file' ? 'active' : ''}" data-sub="file">文件传输审计</button>
        <button class="tab-btn ${sub === 'alarm' ? 'active' : ''}" data-sub="alarm">安全告警</button>
        <button class="tab-btn ${sub === 'login' ? 'active' : ''}" data-sub="login">登录历史</button>
      </div>
      <div class="card">
        <div class="filter-bar">
          <div class="row">
            <input id="audit-kw" class="search-input" value="${esc(state.auditKeyword)}" placeholder="输入设备 ID / 用户名 / IP 搜索...">
            <button id="btn-audit-search" class="btn btn-secondary btn-sm">搜索</button>
          </div>
        </div>
        <div id="audit-table">${renderSkeleton(4)}</div>
      </div>
    `;

    area.querySelectorAll('.tab-btn').forEach((b) => {
      b.onclick = () => {
        state.auditSubTab = b.dataset.sub;
        state.auditPage = 1;
        renderAudit();
      };
    });

    const loadRows = async () => {
      try {
        const res = await api(`/admin/api/audit/${sub}?current=${state.auditPage}&pageSize=${pageSize}&keyword=${encodeURIComponent(state.auditKeyword)}`);
        const rows = res.data || [];
        const total = Number(res.total || 0);
        let html = '';

        if (!rows.length) {
          html = renderEmpty('暂无日志记录', '当前筛选条件下没有对应的审计或登录日志');
        } else if (sub === 'conn') {
          html = `<div class="table-wrap"><table><thead><tr><th>动作</th><th>被控 ID</th><th>主控 ID / 用户</th><th>IP</th><th>备注</th><th>时间</th></tr></thead><tbody>
            ${rows.map((r) => `<tr><td><span class="badge badge-blue">${esc(r.action || '-')}</span></td><td><code>${esc(r.peer_id)}</code></td><td>${esc(r.from_peer)} ${r.from_name ? `<span class="cell-sub">(${esc(r.from_name)})</span>` : ''}</td><td>${esc(r.ip)}</td><td>${esc(r.note || '-')}</td><td title="${esc(fmtTime(r.created_at))}">${esc(relTime(r.created_at))}</td></tr>`).join('')}
          </tbody></table></div>`;
        } else if (sub === 'file') {
          html = `<div class="table-wrap"><table><thead><tr><th>被控 ID</th><th>来源</th><th>路径</th><th>详情</th><th>IP</th><th>时间</th></tr></thead><tbody>
            ${rows.map((r) => `<tr><td><code>${esc(r.peer_id)}</code></td><td>${esc(r.from_peer)} ${r.from_name ? `(${esc(r.from_name)})` : ''}</td><td><code>${esc(r.path)}</code></td><td>${esc(r.info || '-')}</td><td>${esc(r.ip)}</td><td title="${esc(fmtTime(r.created_at))}">${esc(relTime(r.created_at))}</td></tr>`).join('')}
          </tbody></table></div>`;
        } else if (sub === 'alarm') {
          html = `<div class="table-wrap"><table><thead><tr><th>告警类型</th><th>设备 ID</th><th>告警详情</th><th>IP</th><th>时间</th></tr></thead><tbody>
            ${rows.map((r) => `<tr><td><span class="badge badge-red">${esc(r.typ)}</span></td><td><code>${esc(r.peer_id)}</code></td><td>${esc(r.info)}</td><td>${esc(r.ip)}</td><td title="${esc(fmtTime(r.created_at))}">${esc(relTime(r.created_at))}</td></tr>`).join('')}
          </tbody></table></div>`;
        } else {
          html = `<div class="table-wrap"><table><thead><tr><th>用户名</th><th>认证方式</th><th>设备 ID</th><th>状态</th><th>说明</th><th>IP</th><th>时间</th></tr></thead><tbody>
            ${rows.map((r) => `<tr><td><strong>${esc(r.username)}</strong></td><td><span class="badge badge-gray">${esc(r.auth_source)}</span></td><td><code>${esc(r.device_id || '-')}</code></td><td><span class="badge ${r.status === 'success' ? 'badge-green' : 'badge-red'}">${esc(r.status)}</span></td><td>${esc(r.message)}</td><td>${esc(r.ip)}</td><td title="${esc(fmtTime(r.created_at))}">${esc(relTime(r.created_at))}</td></tr>`).join('')}
          </tbody></table></div>`;
        }

        $('audit-table').innerHTML = html + renderPagination(state.auditPage, pageSize, total);
        $('audit-table').querySelectorAll('[data-page]').forEach((btn) => {
          btn.onclick = () => {
            if (btn.disabled) return;
            state.auditPage = Number(btn.dataset.page);
            loadRows();
          };
        });
      } catch (err) {
        $('audit-table').innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
      }
    };

    const doAuditSearch = () => {
      state.auditKeyword = $('audit-kw').value.trim();
      state.auditPage = 1;
      loadRows();
    };
    $('btn-audit-search').onclick = doAuditSearch;
    $('audit-kw').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') doAuditSearch();
    });

    loadRows();
  }

  // ---------------- 7. SSO (OIDC / GitHub / Google / LDAP) ----------------
  async function renderSSO() {
    $('topbar-actions').innerHTML = `<button class="btn btn-primary btn-sm" id="btn-add-oauth">+ 添加 OIDC / OAuth 提供商</button>`;
    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const [oRes, ldapCfg] = await Promise.all([
        api('/admin/api/oauth/providers'),
        api('/admin/api/ldap'),
      ]);
      const providers = oRes.data || [];
      const callbackURL = window.location.origin + '/api/oidc/callback';

      area.innerHTML = `
        <div class="info-banner" style="margin-bottom:16px;">
          <div>
            <strong>统一 OIDC / OAuth 回调地址 (Redirect URI)：</strong>
            <code>${esc(callbackURL)}</code>
          </div>
          <button id="btn-copy-callback" class="btn btn-xs btn-primary">复制回调地址</button>
        </div>

        <div class="card">
          <div class="card-header">
            <h3 class="card-title">OIDC / GitHub / Google 单点登录提供商</h3>
          </div>
          <div class="table-wrap">
            ${providers.length ? `
              <table>
                <thead><tr><th>标识 (op)</th><th>类型</th><th>Client ID</th><th>自动建号</th><th>状态</th><th>操作</th></tr></thead>
                <tbody>
                  ${providers.map((p) => `
                    <tr>
                      <td><strong>${esc(p.name)}</strong></td>
                      <td><span class="badge badge-blue">${esc(p.provider_type)}</span></td>
                      <td><code>${esc(p.client_id)}</code></td>
                      <td>${p.auto_register ? '是' : '否'}</td>
                      <td>${p.enabled ? '<span class="badge badge-green">启用</span>' : '<span class="badge badge-gray">停用</span>'}</td>
                      <td>
                        <div class="row">
                          <button class="btn btn-xs btn-secondary" data-edit-oauth="${p.id}">编辑</button>
                          <button class="btn btn-xs btn-danger" data-del-oauth="${p.id}">删除</button>
                        </div>
                      </td>
                    </tr>`).join('')}
                </tbody>
              </table>` : renderEmpty('暂未配置第三方 SSO 提供商', '支持通用 OIDC、GitHub 与 Google，点击右上角添加')}
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3 class="card-title">LDAP / Active Directory 认证配置</h3></div>
          <form id="form-ldap" class="stack">
            <label class="checkbox-row"><input name="enabled" type="checkbox" ${ldapCfg.enabled ? 'checked' : ''}> 启用 LDAP 登录回退认证</label>
            <div class="grid-2">
              <label class="field"><span>LDAP 地址</span><input name="url" type="text" placeholder="ldap://ldap.example.com:389 或 ldaps://..." value="${esc(ldapCfg.url || '')}"></label>
              <label class="field"><span>Base DN</span><input name="base_dn" type="text" placeholder="dc=example,dc=com" value="${esc(ldapCfg.base_dn || '')}"></label>
              <label class="field"><span>Bind DN</span><input name="bind_dn" type="text" placeholder="cn=readonly,dc=example,dc=com" value="${esc(ldapCfg.bind_dn || '')}"></label>
              <label class="field"><span>Bind 密码 ${ldapCfg.has_bind_password ? '(已设置，留空不变)' : ''}</span><input name="bind_password" type="password" value=""></label>
              <label class="field"><span>用户过滤表达式</span><input name="user_filter" type="text" value="${esc(ldapCfg.user_filter || '(&(objectClass=person)(|(uid=%s)(sAMAccountName=%s)(mail=%s)))')}"></label>
              <label class="field"><span>用户名属性 / 邮箱属性</span>
                <div class="row">
                  <input name="username_attr" type="text" value="${esc(ldapCfg.username_attr || 'uid')}" style="flex:1">
                  <input name="email_attr" type="text" value="${esc(ldapCfg.email_attr || 'mail')}" style="flex:1">
                </div>
              </label>
            </div>
            <div class="row">
              <label class="checkbox-row"><input name="skip_tls_verify" type="checkbox" ${ldapCfg.skip_tls_verify ? 'checked' : ''}> 跳过 LDAPS 自签证书校验</label>
              <label class="checkbox-row"><input name="auto_register" type="checkbox" ${ldapCfg.auto_register !== false ? 'checked' : ''}> 首次登录自动创建账号</label>
            </div>
            <div id="ldap-test-result" class="hidden"></div>
            <div class="row">
              <button type="submit" class="btn btn-primary">保存 LDAP 配置</button>
              <button type="button" id="btn-test-ldap" class="btn btn-secondary">测试 LDAP 连接</button>
            </div>
          </form>
        </div>
      `;

      $('btn-copy-callback').onclick = () => copyText(callbackURL, '已复制回调地址');
      $('btn-add-oauth').onclick = () => openOAuthModal(null, callbackURL);

      area.onclick = async (e) => {
        const editId = e.target.dataset.editOauth;
        const delId = e.target.dataset.delOauth;
        if (editId) {
          const p = providers.find((x) => String(x.id) === editId);
          if (p) openOAuthModal(p, callbackURL);
        } else if (delId) {
          const ok = await confirmDialog({
            title: '删除 SSO 提供商',
            message: '确定删除该 OIDC/OAuth 配置吗？',
            confirmText: '确认删除',
          });
          if (ok) {
            await api('/admin/api/oauth/providers/' + delId, { method: 'DELETE' });
            toast('已删除');
            renderSSO();
          }
        }
      };

      const readLDAPForm = (form) => {
        const fd = new FormData(form);
        return {
          enabled: Boolean(fd.get('enabled')),
          url: fd.get('url'),
          base_dn: fd.get('base_dn'),
          bind_dn: fd.get('bind_dn'),
          bind_password: fd.get('bind_password'),
          user_filter: fd.get('user_filter'),
          username_attr: fd.get('username_attr'),
          email_attr: fd.get('email_attr'),
          skip_tls_verify: Boolean(fd.get('skip_tls_verify')),
          auto_register: Boolean(fd.get('auto_register')),
        };
      };

      $('form-ldap').onsubmit = async (ev) => {
        ev.preventDefault();
        try {
          await api('/admin/api/ldap', { method: 'PUT', body: JSON.stringify(readLDAPForm(ev.target)) });
          toast('LDAP 配置已保存');
        } catch (err) {
          toast(err.message, false);
        }
      };

      $('btn-test-ldap').onclick = async () => {
        const resBox = $('ldap-test-result');
        resBox.className = 'info-banner';
        resBox.textContent = '正在测试 LDAP 连通性...';
        try {
          const res = await api('/admin/api/ldap/test', { method: 'POST', body: JSON.stringify(readLDAPForm($('form-ldap'))) });
          resBox.className = 'success-banner';
          resBox.textContent = res.message || 'LDAP 连接与 Bind 成功';
        } catch (err) {
          resBox.className = 'error-banner';
          resBox.textContent = '测试失败：' + err.message;
        }
      };
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  function openOAuthModal(p, defaultRedirect) {
    const isEdit = Boolean(p);
    openModal(
      isEdit ? '编辑 SSO 提供商: ' + p.name : '添加 OIDC / OAuth 提供商',
      `<form id="form-oauth" class="stack">
        <label class="field"><span>名称标识 (例如 github / google / okta)</span><input name="name" type="text" value="${esc(p?.name || '')}" required></label>
        <label class="field"><span>提供商类型</span>
          <select id="oauth-type-select" name="provider_type">
            <option value="oidc" ${!p || p.provider_type === 'oidc' ? 'selected' : ''}>通用 OIDC (Keycloak / Authentik / Authelia / Okta)</option>
            <option value="github" ${p?.provider_type === 'github' ? 'selected' : ''}>GitHub OAuth</option>
            <option value="google" ${p?.provider_type === 'google' ? 'selected' : ''}>Google OAuth</option>
          </select>
        </label>
        <label class="field"><span>Client ID</span><input name="client_id" type="text" value="${esc(p?.client_id || '')}" required></label>
        <label class="field"><span>Client Secret ${p?.has_secret ? '(留空保持不变)' : ''}</span><input name="client_secret" type="password"></label>
        <label id="oauth-issuer-field" class="field"><span>OIDC Issuer 地址（支持自动发现 .well-known）</span><input name="issuer" type="text" value="${esc(p?.issuer || '')}" placeholder="https://auth.example.com/application/o/rustdesk/"></label>
        <label class="field"><span>自定义 Redirect URI（留空自动使用 ${esc(defaultRedirect)}）</span><input name="redirect_url" type="text" value="${esc(p?.redirect_url || '')}"></label>
        <label class="field"><span>Scopes（可选）</span><input name="scopes" type="text" value="${esc(p?.scopes || '')}" placeholder="openid email profile"></label>
        <div class="row">
          <label class="checkbox-row"><input name="enabled" type="checkbox" ${!p || p.enabled ? 'checked' : ''}> 启用</label>
          <label class="checkbox-row"><input name="auto_register" type="checkbox" ${!p || p.auto_register ? 'checked' : ''}> 首次登录自动建号</label>
        </div>
        <button class="btn btn-primary" type="submit">保存</button>
      </form>`,
      (root) => {
        const typeSel = root.querySelector('#oauth-type-select');
        const issuerField = root.querySelector('#oauth-issuer-field');
        const toggleIssuer = () => {
          issuerField.classList.toggle('hidden', typeSel.value !== 'oidc');
        };
        toggleIssuer();
        typeSel.addEventListener('change', toggleIssuer);

        root.querySelector('#form-oauth').addEventListener('submit', async (ev) => {
          ev.preventDefault();
          const fd = new FormData(ev.target);
          const payload = {
            name: fd.get('name'),
            provider_type: fd.get('provider_type'),
            client_id: fd.get('client_id'),
            client_secret: fd.get('client_secret'),
            issuer: fd.get('issuer'),
            redirect_url: fd.get('redirect_url'),
            scopes: fd.get('scopes'),
            enabled: Boolean(fd.get('enabled')),
            auto_register: Boolean(fd.get('auto_register')),
          };
          try {
            if (isEdit) {
              await api('/admin/api/oauth/providers/' + p.id, { method: 'PUT', body: JSON.stringify(payload) });
            } else {
              await api('/admin/api/oauth/providers', { method: 'POST', body: JSON.stringify(payload) });
            }
            closeModal();
            toast('SSO 提供商已保存');
            renderSSO();
          } catch (err) {
            toast(err.message, false);
          }
        });
      }
    );
  }

  // ---------------- 8. Server Config & Official Updates ----------------
  async function renderSystem(forceRefresh = false) {
    $('topbar-actions').innerHTML = `<button class="btn btn-primary btn-sm" id="btn-check-updates">检查官方最新版本</button>`;
    $('btn-check-updates').onclick = () => renderSystem(true);

    const area = $('content-area');
    area.innerHTML = renderSkeleton(5);

    try {
      const [cfg, upd] = await Promise.all([
        api('/admin/api/system/server-config'),
        api('/admin/api/system/updates' + (forceRefresh ? '?refresh=1' : '')),
      ]);
      const clientRel = upd.client_release || {};
      const serverRel = upd.server_release || {};

      area.innerHTML = `
        <div class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">服务端参数、二维码与客户端导入配置</h3>
              <div class="cell-sub">公钥路径: <code>${esc(cfg.key_file_path)}</code> (${esc(cfg.key_source)})</div>
            </div>
            <span class="badge ${cfg.webclient_ready ? 'badge-green' : 'badge-gray'}">${cfg.webclient_ready ? 'Web 客户端已托管 (/webclient/)' : 'Web 客户端未同步'}</span>
          </div>

          <div class="stack">
            <div class="grid-2" style="align-items:center;">
              <div class="kv-grid">
                <div class="kv-item"><span class="kv-key">ID 服务器</span><span class="kv-val">${esc(cfg.id_server || '-')}</span><button class="btn btn-xs btn-secondary" data-copy-val="${esc(cfg.id_server)}">复制</button></div>
                <div class="kv-item"><span class="kv-key">中继服务器</span><span class="kv-val">${esc(cfg.relay_server || '-')}</span><button class="btn btn-xs btn-secondary" data-copy-val="${esc(cfg.relay_server)}">复制</button></div>
                <div class="kv-item"><span class="kv-key">API 服务器</span><span class="kv-val">${esc(cfg.api_server || '-')}</span><button class="btn btn-xs btn-secondary" data-copy-val="${esc(cfg.api_server)}">复制</button></div>
                <div class="kv-item"><span class="kv-key">Public Key</span><span class="kv-val">${esc(cfg.public_key || '未检测到公钥')}</span><button class="btn btn-xs btn-secondary" id="btn-copy-key">复制</button></div>
              </div>
              <div class="qr-panel">
                ${cfg.qr_data_uri ? `<div class="qr-frame"><img src="${esc(cfg.qr_data_uri)}" alt="RustDesk 配置二维码"></div>` : ''}
                <span class="cell-sub">手机端「网络设置」扫码即可完成全部配置</span>
              </div>
            </div>

            <div>
              <div class="row" style="justify-content:space-between;margin-bottom:6px;">
                <span class="stat-label">RustDesk 客户端一键配置字符串（直接粘贴导入到桌面/移动端网络配置）</span>
                <button class="btn btn-xs btn-primary" id="btn-copy-cfg">复制配置串</button>
              </div>
              <div class="code-box">${esc(cfg.config_string || '-')}</div>
            </div>

            <div>
              <div class="row" style="justify-content:space-between;margin-bottom:6px;">
                <span class="stat-label">CLI 静默部署指令</span>
                <button class="btn btn-xs btn-secondary" id="btn-copy-cli">复制命令</button>
              </div>
              <div class="code-box">${esc(cfg.cli_command || '-')}</div>
            </div>
          </div>
        </div>

        <div class="grid-2">
          <div class="card">
            <div class="card-header">
              <div>
                <h3 class="card-title">官方服务端 (rustdesk/rustdesk-server)</h3>
                <div class="cell-sub">Docker 镜像: <code>${esc(upd.docker_image)}</code></div>
              </div>
              <span class="badge badge-green">${esc(serverRel.tag_name || 'latest')}</span>
            </div>
            <p class="cell-sub" style="margin-top:0;">发布时间: ${esc(fmtTime(serverRel.published_at))} ${upd.cached ? '(缓存)' : '(实时)'}</p>
            ${serverRel.error ? `<div class="error-banner" style="margin-bottom:10px;">${esc(serverRel.error)}</div>` : ''}
            <div class="stack">
              <div>
                <div class="row" style="justify-content:space-between;margin-bottom:5px;">
                  <span class="stat-label">仅更新官方 hbbs & hbbr</span>
                  <button class="btn btn-xs btn-secondary" data-copy-val="./scripts/update.sh">复制</button>
                </div>
                <div class="code-box">./scripts/update.sh</div>
              </div>
              <div>
                <div class="row" style="justify-content:space-between;margin-bottom:5px;">
                  <span class="stat-label">拉取 GitHub 代码 + 重建 API + 更新官方服务端</span>
                  <button class="btn btn-xs btn-secondary" data-copy-val="./scripts/update.sh --all">复制</button>
                </div>
                <div class="code-box">./scripts/update.sh --all</div>
              </div>
            </div>
            <p style="margin:12px 0 0;"><a href="${esc(serverRel.html_url)}" target="_blank" rel="noopener">查看 GitHub 官方服务端 Release ↗</a></p>
          </div>

          <div class="card">
            <div class="card-header">
              <div>
                <h3 class="card-title">官方客户端 (rustdesk/rustdesk)</h3>
                <div class="cell-sub">发布时间: ${esc(fmtTime(clientRel.published_at))}</div>
              </div>
              <span class="badge badge-blue">${esc(clientRel.tag_name || '-')}</span>
            </div>
            ${clientRel.error ? `<div class="error-banner" style="margin-bottom:10px;">${esc(clientRel.error)}</div>` : ''}
            <div class="table-wrap" style="max-height:260px;overflow-y:auto;">
              <table>
                <thead><tr><th>官方安装包</th><th>大小</th><th>下载</th></tr></thead>
                <tbody>
                  ${(clientRel.assets || []).map((a) => `
                    <tr>
                      <td>${esc(a.name)}</td>
                      <td>${esc(fmtBytes(a.size))}</td>
                      <td><a href="${esc(a.url)}" target="_blank" rel="noopener">下载</a></td>
                    </tr>`).join('') || '<tr><td colspan="3">无法连接 GitHub API 或暂无列表</td></tr>'}
                </tbody>
              </table>
            </div>
            <p style="margin:12px 0 0;"><a href="${esc(clientRel.html_url)}" target="_blank" rel="noopener">前往 GitHub 下载全部官方客户端 ↗</a></p>
          </div>
        </div>
      `;

      $('btn-copy-key').onclick = () => copyText(cfg.public_key, '已复制官方 Public Key');
      $('btn-copy-cfg').onclick = () => copyText(cfg.config_string, '已复制一键配置字符串');
      $('btn-copy-cli').onclick = () => copyText(cfg.cli_command, '已复制 CLI 命令');
      area.querySelectorAll('[data-copy-val]').forEach((btn) => {
        btn.onclick = () => copyText(btn.dataset.copyVal);
      });
    } catch (err) {
      area.innerHTML = `<div class="error-banner">${esc(err.message)}</div>`;
    }
  }

  boot();
})();
