const state = {
  me: null,
  projects: [],
  currentProject: null,
  users: [],
  auditLogs: [],
  route: "dashboard",
  message: "",
  error: "",
};

function isAdmin() {
  return !!state.me && state.me.roles.includes("admin");
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (!response.ok) {
    let data = {};
    try { data = await response.json(); } catch (_) {}
    throw new Error(data.error || `Request failed: ${response.status}`);
  }
  const contentType = response.headers.get("content-type") || "";
  return contentType.includes("application/json") ? response.json() : response.text();
}

function setMessage(message, isError = false) {
  state.message = isError ? "" : message;
  state.error = isError ? message : "";
  render();
}

function loginView() {
  return `
    <div class="login-wrap">
      <div class="login-panel">
        <h1>Prototype Hub</h1>
        <p class="muted">登录后管理原型项目、上传 ZIP 并直接访问 HTML 原型。</p>
        <form id="login-form">
          <label>用户名</label>
          <input name="username" placeholder="admin" required />
          <label style="margin-top:12px;">密码</label>
          <input name="password" type="password" required />
          <button style="margin-top:16px; width:100%;">登录</button>
          ${state.error ? `<div class="message error">${escapeHtml(state.error)}</div>` : ""}
          ${state.message ? `<div class="message">${escapeHtml(state.message)}</div>` : ""}
        </form>
      </div>
    </div>
  `;
}

function dashboardView() {
  return `
    <section class="card">
      <div class="section-title">
        <div>
          <h2 style="margin:0;">项目列表</h2>
          <div class="muted">查看自己拥有权限的原型项目，并上传 ZIP 新版本。</div>
        </div>
        <button id="reload-projects-btn" class="secondary">刷新</button>
      </div>
      <div class="grid">
        ${state.projects.map(project => `
          <article class="project-card">
            <h3>${escapeHtml(project.name)}</h3>
            <div class="muted">${escapeHtml(project.description || "暂无描述")}</div>
            <div style="margin:10px 0;">
              <span class="tag">${escapeHtml(project.status)}</span>
              ${project.currentVersion ? `<span class="tag warn">当前 v${project.currentVersion.versionNo}</span>` : `<span class="tag error">未发布</span>`}
            </div>
            <button class="open-project-btn" data-project-id="${project.id}">进入项目</button>
          </article>
        `).join("")}
      </div>
    </section>
    ${isAdmin() ? `
      <section class="card">
        <h2 style="margin-top:0;">创建项目</h2>
        <form id="create-project-form">
          <div class="row">
            <div>
              <label>项目名称</label>
              <input name="name" required />
            </div>
            <div>
              <label>项目描述</label>
              <input name="description" />
            </div>
          </div>
          <button style="margin-top:14px;">创建项目</button>
        </form>
      </section>
    ` : ""}
  `;
}

function projectView() {
  const project = state.currentProject;
  if (!project) return `<section class="card">项目不存在或还未加载。</section>`;
  return `
    <section class="card">
      <div class="section-title">
        <div>
          <h2 style="margin:0;">${escapeHtml(project.name)}</h2>
          <div class="muted">${escapeHtml(project.description || "暂无描述")}</div>
        </div>
        <div class="row" style="justify-content:flex-end;">
          <button class="secondary" id="back-dashboard-btn">返回</button>
          ${project.currentVersionId ? `<a href="/preview/${project.currentVersionId}" target="_blank"><button>打开当前预览</button></a>` : ""}
          ${project.slug ? `<a href="/p/${project.slug}" target="_blank"><button class="ghost">快捷访问</button></a>` : ""}
        </div>
      </div>
      <div class="row">
        <div class="card" style="padding:16px;">
          <h3 style="margin-top:0;">项目信息</h3>
          <div>Slug: <strong>${escapeHtml(project.slug)}</strong></div>
          <div class="muted">Owner: ${escapeHtml(project.owner?.displayName || "-")}</div>
          <div class="muted">当前版本: ${project.currentVersion ? `v${project.currentVersion.versionNo}` : "未设置"}</div>
        </div>
        <div class="card" style="padding:16px;">
          <h3 style="margin-top:0;">上传新版本</h3>
          <form id="upload-version-form">
            <label>ZIP 文件</label>
            <input type="file" name="file" accept=".zip" required />
            <button style="margin-top:12px;">上传并发布</button>
          </form>
        </div>
      </div>
    </section>
    <section class="card">
      <h2 style="margin-top:0;">版本历史</h2>
      <div class="content">
        ${(project.versions || []).map(version => `
          <div class="version-item">
            <div class="section-title">
              <div>
                <strong>v${version.versionNo}</strong>
                <span class="tag ${version.status === "failed" ? "error" : version.status === "processing" ? "warn" : ""}">${escapeHtml(version.status)}</span>
                ${project.currentVersionId === version.id ? `<span class="tag">当前版本</span>` : ""}
              </div>
              <div class="row" style="justify-content:flex-end;">
                ${version.status === "ready" ? `<a href="/preview/${version.id}" target="_blank"><button class="ghost">预览</button></a>` : ""}
                ${version.status === "ready" && project.currentVersionId !== version.id ? `<button class="switch-version-btn" data-version-id="${version.id}">设为当前</button>` : ""}
              </div>
            </div>
            <div class="muted">入口文件: ${escapeHtml(version.entryFile)}</div>
            ${version.status === "ready" && (version.htmlFiles || []).length ? `
              <div class="row" style="margin-top:12px; align-items:end;">
                <div>
                  <label>入口页</label>
                  <select class="entry-file-select" data-version-id="${version.id}">
                    ${(version.htmlFiles || []).map(file => `<option value="${escapeHtml(file)}" ${file === version.entryFile ? "selected" : ""}>${escapeHtml(file)}</option>`).join("")}
                  </select>
                </div>
                <div style="flex:0 0 auto;">
                  <button class="secondary save-entry-file-btn" data-version-id="${version.id}">保存入口页</button>
                </div>
              </div>
            ` : ""}
            ${version.errorMessage ? `<pre class="message error">${escapeHtml(version.errorMessage)}</pre>` : ""}
          </div>
        `).join("")}
      </div>
    </section>
  `;
}

function usersView() {
  return `
    <section class="card">
      <h2 style="margin-top:0;">用户管理</h2>
      <form id="create-user-form">
        <div class="row">
          <div><label>用户名</label><input name="username" required /></div>
          <div><label>显示名称</label><input name="displayName" required /></div>
          <div><label>初始密码</label><input name="password" type="password" required /></div>
          <div>
            <label>角色</label>
            <select name="roles" multiple size="3">
              <option value="admin">admin</option>
              <option value="user" selected>user</option>
              <option value="viewer">viewer</option>
            </select>
          </div>
        </div>
        <button style="margin-top:14px;">创建用户</button>
      </form>
    </section>
    <section class="card">
      <table>
        <thead><tr><th>用户</th><th>状态</th><th>角色</th><th>操作</th></tr></thead>
        <tbody>
          ${state.users.map(user => `
            <tr>
              <td><strong>${escapeHtml(user.displayName)}</strong><br /><span class="muted">${escapeHtml(user.username)}</span></td>
              <td>${escapeHtml(user.status)}</td>
              <td>
                <select class="role-select" data-user-id="${user.id}" multiple size="3">
                  ${["admin", "user", "viewer"].map(code => `<option value="${code}" ${(user.roles || []).some(role => role.code === code) ? "selected" : ""}>${code}</option>`).join("")}
                </select>
              </td>
              <td>
                <div class="row">
                  <button class="secondary save-roles-btn" data-user-id="${user.id}">保存角色</button>
                  <button class="${user.status === "active" ? "danger" : "secondary"} user-status-btn" data-user-id="${user.id}" data-next-status="${user.status === "active" ? "disabled" : "active"}">${user.status === "active" ? "禁用" : "启用"}</button>
                </div>
              </td>
            </tr>
          `).join("")}
        </tbody>
      </table>
    </section>
  `;
}

function auditView() {
  return `
    <section class="card">
      <div class="section-title">
        <div>
          <h2 style="margin:0;">审计日志</h2>
          <div class="muted">记录登录、用户管理、版本上传和切换等关键动作。</div>
        </div>
        <button id="reload-audit-btn" class="secondary">刷新</button>
      </div>
      <table>
        <thead><tr><th>时间</th><th>动作</th><th>操作人</th><th>对象</th><th>详情</th></tr></thead>
        <tbody>
          ${state.auditLogs.map(log => `
            <tr>
              <td>${escapeHtml(formatDate(log.createdAt))}</td>
              <td>${escapeHtml(log.action)}</td>
              <td>${escapeHtml(log.actor?.displayName || "-")}</td>
              <td>${escapeHtml(`${log.targetType}#${log.targetId}`)}</td>
              <td>${escapeHtml(log.detail || "")}</td>
            </tr>
          `).join("")}
        </tbody>
      </table>
    </section>
  `;
}

function mainView() {
  return `
    <div class="shell">
      <div class="hero">
        <div>
          <h1>Prototype Hub</h1>
          <p>账号、权限、ZIP 发布与 HTML 预览都在一个单体应用里完成。</p>
        </div>
        <div>
          <strong>${escapeHtml(state.me.displayName)}</strong>
          <div class="muted">${state.me.username} · ${state.me.roles.join(", ")}</div>
        </div>
      </div>
      <div class="layout">
        <aside class="sidebar">
          <button data-route="dashboard">项目工作台</button>
          ${state.me.roles.includes("admin") ? `<button class="secondary" data-route="users">用户管理</button>` : ""}
          ${state.me.roles.includes("admin") ? `<button class="secondary" data-route="audit">审计日志</button>` : ""}
          <button class="ghost" id="logout-btn">退出登录</button>
          ${state.error ? `<div class="message error">${escapeHtml(state.error)}</div>` : ""}
          ${state.message ? `<div class="message">${escapeHtml(state.message)}</div>` : ""}
        </aside>
        <main class="content">
          ${state.route === "dashboard" ? dashboardView() : ""}
          ${state.route === "project" ? projectView() : ""}
          ${state.route === "users" ? usersView() : ""}
          ${state.route === "audit" ? auditView() : ""}
        </main>
      </div>
    </div>
  `;
}

function render() {
  const app = document.getElementById("app");
  app.innerHTML = state.me ? mainView() : loginView();
  state.me ? bindShell() : bindLogin();
}

function bindLogin() {
  document.getElementById("login-form").addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.target);
    try {
      setMessage("正在登录...");
      await api("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ username: form.get("username"), password: form.get("password") }),
      });
      await bootstrap();
      setMessage("登录成功");
    } catch (error) {
      setMessage(error.message, true);
    }
  });
}

function bindShell() {
  document.querySelectorAll("[data-route]").forEach(button => {
    button.addEventListener("click", async () => {
      state.route = button.dataset.route;
      if (state.route === "users") await loadUsers();
      if (state.route === "audit") await loadAudit();
      render();
    });
  });

  const logoutBtn = document.getElementById("logout-btn");
  if (logoutBtn) logoutBtn.addEventListener("click", async () => {
    await api("/api/auth/logout", { method: "POST" });
    Object.assign(state, { me: null, projects: [], currentProject: null, route: "dashboard" });
    render();
  });

  const createProjectForm = document.getElementById("create-project-form");
  if (createProjectForm) createProjectForm.addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.target);
    try {
      await api("/api/projects", {
        method: "POST",
        body: JSON.stringify({ name: form.get("name"), description: form.get("description") }),
      });
      event.target.reset();
      setMessage("项目已创建");
      await loadProjects();
    } catch (error) {
      setMessage(error.message, true);
    }
  });

  const reloadProjectsBtn = document.getElementById("reload-projects-btn");
  if (reloadProjectsBtn) reloadProjectsBtn.addEventListener("click", loadProjects);

  document.querySelectorAll(".open-project-btn").forEach(button => {
    button.addEventListener("click", async () => {
      try { await openProject(button.dataset.projectId); } catch (error) { setMessage(error.message, true); }
    });
  });

  const backBtn = document.getElementById("back-dashboard-btn");
  if (backBtn) backBtn.addEventListener("click", async () => {
    state.route = "dashboard";
    state.currentProject = null;
    await loadProjects();
    render();
  });

  const uploadForm = document.getElementById("upload-version-form");
  if (uploadForm) uploadForm.addEventListener("submit", async event => {
    event.preventDefault();
    const formData = new FormData(event.target);
    try {
      setMessage("ZIP 已提交，后台正在处理...");
      const response = await fetch(`/api/projects/${state.currentProject.id}/versions/upload`, { method: "POST", credentials: "include", body: formData });
      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || "upload failed");
      }
      event.target.reset();
      await pollProjectUntilReady(state.currentProject.id);
    } catch (error) {
      setMessage(error.message, true);
    }
  });

  document.querySelectorAll(".switch-version-btn").forEach(button => {
    button.addEventListener("click", async () => {
      try {
        await api(`/api/projects/${state.currentProject.id}/current-version`, {
          method: "POST",
          body: JSON.stringify({ versionId: Number(button.dataset.versionId) }),
        });
        setMessage("当前版本已切换");
        await openProject(state.currentProject.id);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  document.querySelectorAll(".save-entry-file-btn").forEach(button => {
    button.addEventListener("click", async () => {
      const select = document.querySelector(`.entry-file-select[data-version-id="${button.dataset.versionId}"]`);
      try {
        await api(`/api/projects/${state.currentProject.id}/versions/${button.dataset.versionId}/entry-file`, {
          method: "POST",
          body: JSON.stringify({ entryFile: select.value }),
        });
        setMessage("入口页已更新");
        await openProject(state.currentProject.id);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  const createUserForm = document.getElementById("create-user-form");
  if (createUserForm) createUserForm.addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.target);
    const select = event.target.querySelector('select[name="roles"]');
    const roles = Array.from(select.selectedOptions).map(option => option.value);
    try {
      await api("/api/admin/users", {
        method: "POST",
        body: JSON.stringify({
          username: form.get("username"),
          displayName: form.get("displayName"),
          password: form.get("password"),
          roles,
        }),
      });
      event.target.reset();
      setMessage("用户已创建");
      await loadUsers();
    } catch (error) {
      setMessage(error.message, true);
    }
  });

  document.querySelectorAll(".user-status-btn").forEach(button => {
    button.addEventListener("click", async () => {
      try {
        await api(`/api/admin/users/${button.dataset.userId}/status`, {
          method: "PATCH",
          body: JSON.stringify({ status: button.dataset.nextStatus }),
        });
        setMessage("用户状态已更新");
        await loadUsers();
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  document.querySelectorAll(".save-roles-btn").forEach(button => {
    button.addEventListener("click", async () => {
      const select = document.querySelector(`.role-select[data-user-id="${button.dataset.userId}"]`);
      const roles = Array.from(select.selectedOptions).map(option => option.value);
      try {
        await api(`/api/admin/users/${button.dataset.userId}/roles`, {
          method: "POST",
          body: JSON.stringify({ roles }),
        });
        setMessage("用户角色已更新");
        await loadUsers();
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  const reloadAuditBtn = document.getElementById("reload-audit-btn");
  if (reloadAuditBtn) reloadAuditBtn.addEventListener("click", loadAudit);
}

async function bootstrap() {
  const me = await api("/api/me");
  state.me = me.user;
  state.route = "dashboard";
  await loadProjects();
  if (state.me.roles.includes("admin")) {
    await loadUsers();
    await loadAudit();
  }
  render();
}

async function loadProjects() {
  const data = await api("/api/projects");
  state.projects = data.items || [];
  if (state.route === "dashboard") render();
}

async function openProject(projectId) {
  const data = await api(`/api/projects/${projectId}`);
  state.currentProject = data.project;
  state.route = "project";
  render();
}

async function pollProjectUntilReady(projectId) {
  for (let index = 0; index < 10; index += 1) {
    await sleep(1500);
    const data = await api(`/api/projects/${projectId}`);
    state.currentProject = data.project;
    render();
    const latest = (data.project.versions || [])[0];
    if (!latest || latest.status !== "processing") {
      if (latest?.status === "ready") setMessage("版本处理完成");
      else setMessage(`版本处理失败：${latest?.errorMessage || "unknown error"}`, true);
      return;
    }
  }
  setMessage("版本仍在处理中，请稍后刷新项目页面。");
}

async function loadUsers() {
  const data = await api("/api/admin/users");
  state.users = data.items || [];
  if (state.route === "users") render();
}

async function loadAudit() {
  const data = await api("/api/admin/audit-logs");
  state.auditLogs = data.items || [];
  if (state.route === "audit") render();
}

function escapeHtml(input) {
  return String(input || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function formatDate(value) {
  try { return new Date(value).toLocaleString(); } catch (_) { return value || ""; }
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

(async function init() {
  try { await bootstrap(); } catch (_) { render(); }
})();
