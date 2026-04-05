const state = {
  me: null,
  projects: [],
  currentProject: null,
  users: [],
  memberCandidates: [],
  auditLogs: [],
  route: "dashboard",
  message: "",
  error: "",
  busy: null,
  projectEditor: null,
  versionEditor: null,
  permissionEditor: null,
  auditPagination: {
    page: 1,
    pageSize: 20,
    total: 0,
  },
  projectFilters: {
    keyword: "",
    status: "all",
    release: "all",
  },
};

async function api(url, options = {}) {
  const response = await fetch(url, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });

  if (!response.ok) {
    let data = {};
    try {
      data = await response.json();
    } catch (_) {}
    throw new Error(data.error || `Request failed: ${response.status}`);
  }

  const contentType = response.headers.get("content-type") || "";
  return contentType.includes("application/json") ? response.json() : response.text();
}

function hasRole(role) {
  return !!state.me && state.me.roles.includes(role);
}

function isAdmin() {
  return hasRole("admin");
}

function canManageProjects() {
  return hasRole("admin") || hasRole("project_admin");
}

function canManageUsers() {
  return hasRole("admin");
}

function canViewAudit() {
  return hasRole("admin") || hasRole("project_admin");
}

function navButtonClass(route, variant = "") {
  const classes = [];
  if (variant) {
    classes.push(variant);
  }
  if (state.route === route || (route === "dashboard" && state.route === "project")) {
    classes.push("active");
  }
  return classes.join(" ");
}

function escapeHtml(input) {
  return String(input || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function setMessage(message, isError = false) {
  state.message = isError ? "" : message;
  state.error = isError ? message : "";
  render();
}

function beginBusy(text) {
  state.busy = text || "处理中...";
  render();
}

function endBusy() {
  state.busy = null;
  render();
}

function busyDisabledAttr() {
  return state.busy ? "disabled" : "";
}

function upsertProject(project) {
  if (!project) {
    return;
  }
  const index = state.projects.findIndex(item => item.id === project.id);
  if (index >= 0) {
    state.projects[index] = { ...state.projects[index], ...project };
    return;
  }
  state.projects.unshift(project);
}

function formatDate(value) {
  try {
    return new Date(value).toLocaleString();
  } catch (_) {
    return value || "";
  }
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function openVersionEditor(type, versionId) {
  const data = await api(`/api/projects/${state.currentProject.id}/versions/${versionId}`);
  state.versionEditor = {
    type,
    version: data.version,
  };
  render();
}

function closeVersionEditor() {
  state.versionEditor = null;
  render();
}

function openProjectEditor() {
  if (!state.currentProject) return;
  state.projectEditor = {
    name: state.currentProject.name || "",
    description: state.currentProject.description || "",
    status: state.currentProject.status || "active",
  };
  render();
}

function closeProjectEditor() {
  state.projectEditor = null;
  render();
}

function projectAssignableUsers(project) {
  const assigned = new Set((project.members || []).map(member => member.userId));
  return state.memberCandidates
    .map(user => ({
      id: user.id,
      username: user.username,
      displayName: user.displayName,
      assigned: assigned.has(user.id),
    }));
}

function openPermissionEditor() {
  if (!state.currentProject) return;
  state.permissionEditor = {
    available: projectAssignableUsers(state.currentProject).filter(user => !user.assigned),
    selected: projectAssignableUsers(state.currentProject).filter(user => user.assigned),
    availablePicked: [],
    selectedPicked: [],
  };
  render();
}

function closePermissionEditor() {
  state.permissionEditor = null;
  render();
}

function updatePermissionSelection(listName, values) {
  if (!state.permissionEditor) return;
  state.permissionEditor[listName] = values.map(value => Number(value));
}

function movePermissionItems(fromKey, toKey, selectionKey) {
  if (!state.permissionEditor) return;
  const moving = new Set(state.permissionEditor[selectionKey]);
  if (!moving.size) return;
  const movingItems = state.permissionEditor[fromKey].filter(item => moving.has(item.id));
  state.permissionEditor[fromKey] = state.permissionEditor[fromKey].filter(item => !moving.has(item.id));
  state.permissionEditor[toKey] = [...state.permissionEditor[toKey], ...movingItems].sort((a, b) => a.displayName.localeCompare(b.displayName, "zh-CN"));
  state.permissionEditor.availablePicked = [];
  state.permissionEditor.selectedPicked = [];
  render();
}

function permissionMembersSummaryView(project) {
  const members = (project.members || [])
    .filter(member => member.user && member.user.username !== "admin")
    .map(member => `${member.user.displayName} (${member.user.username})`);
  return members.length ? members.join(" / ") : "暂无授权用户";
}

function loginView() {
  return `
    <div class="login-wrap">
      <div class="login-panel">
        <h1>Prototype Hub</h1>
        <p class="muted">管理员管理项目、权限和版本，项目管理员可以管理项目，普通用户只查看和预览自己有权限的原型。</p>
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

function filterProjects(items) {
  const keyword = state.projectFilters.keyword.trim().toLowerCase();
  return (items || []).filter(project => {
    const matchesKeyword = !keyword || [project.name, project.slug, project.description, project.owner?.displayName]
      .filter(Boolean)
      .some(value => String(value).toLowerCase().includes(keyword));

    const matchesStatus = state.projectFilters.status === "all" || project.status === state.projectFilters.status;

    const hasCurrentVersion = !!project.currentVersion;
    const matchesRelease = state.projectFilters.release === "all"
      || (state.projectFilters.release === "published" && hasCurrentVersion)
      || (state.projectFilters.release === "unpublished" && !hasCurrentVersion);

    return matchesKeyword && matchesStatus && matchesRelease;
  });
}

function projectRowsView(projects) {
  if (!projects.length) {
    return `
      <tr>
        <td colspan="${isAdmin() ? 7 : 6}" class="muted">没有符合筛选条件的项目。</td>
      </tr>
    `;
  }

  return projects.map(project => `
    <tr>
      <td>
        <strong>${escapeHtml(project.name)}</strong>
        <div class="muted">${escapeHtml(project.slug)}</div>
      </td>
      <td>${escapeHtml(project.description || "-")}</td>
      <td>${escapeHtml(project.owner?.displayName || "-")}</td>
      <td><span class="tag">${escapeHtml(project.status || "-")}</span></td>
      <td>
        ${project.currentVersion ? `
          <span class="tag warn">v${project.currentVersion.versionNo}</span>
          <div class="muted">${escapeHtml(project.currentVersion.entryFile || "")}</div>
        ` : `<span class="tag error">未发布</span>`}
      </td>
        <td>${escapeHtml(formatDate(project.createdAt))}</td>
      <td class="actions-cell">
        <div class="project-actions">
          <button class="open-project-btn" data-project-id="${project.id}" ${busyDisabledAttr()}>查看</button>
          ${project.currentVersionId ? `<a href="/preview/${project.currentVersionId}" target="_blank"><button type="button" class="ghost">预览</button></a>` : ""}
          ${isAdmin() ? `<button class="danger delete-project-btn" data-project-id="${project.id}" data-project-name="${escapeHtml(project.name)}" ${busyDisabledAttr()}>${state.busy ? "删除中..." : "删除"}</button>` : ""}
        </div>
      </td>
    </tr>
  `).join("");
}

function dashboardView() {
  const filteredProjects = filterProjects(state.projects);

  return `
    <section class="card">
      <div class="section-title">
        <div>
          <h2 style="margin:0;">项目列表</h2>
          <div class="muted">列表展示支持前端筛选。普通用户只能查看和预览自己有权限的项目。</div>
        </div>
        <button id="reload-projects-btn" class="secondary">刷新</button>
      </div>

      <div class="row">
        <div>
          <label>关键词</label>
          <input id="project-filter-keyword" placeholder="按项目名、Slug、描述、负责人筛选" value="${escapeHtml(state.projectFilters.keyword)}" />
        </div>
        <div>
          <label>状态</label>
          <select id="project-filter-status">
            <option value="all" ${state.projectFilters.status === "all" ? "selected" : ""}>全部</option>
            <option value="active" ${state.projectFilters.status === "active" ? "selected" : ""}>active</option>
            <option value="archived" ${state.projectFilters.status === "archived" ? "selected" : ""}>archived</option>
          </select>
        </div>
        <div>
          <label>发布状态</label>
          <select id="project-filter-release">
            <option value="all" ${state.projectFilters.release === "all" ? "selected" : ""}>全部</option>
            <option value="published" ${state.projectFilters.release === "published" ? "selected" : ""}>已发布</option>
            <option value="unpublished" ${state.projectFilters.release === "unpublished" ? "selected" : ""}>未发布</option>
          </select>
        </div>
      </div>

      <div class="muted" style="margin:14px 0 10px;">共 ${state.projects.length} 个项目，当前筛选结果 ${filteredProjects.length} 个。</div>

      <table>
        <thead>
          <tr>
            <th>项目</th>
            <th>描述</th>
            <th>负责人</th>
            <th>状态</th>
            <th>当前版本</th>
            <th>创建时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>${projectRowsView(filteredProjects)}</tbody>
      </table>
    </section>

    ${canManageProjects() ? `
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

function projectPermissionsView(project) {
  if (!canManageProjects()) return "";

  return `
    <section class="card">
      <div class="section-title">
        <div>
          <h2 style="margin:0;">项目权限</h2>
          <div class="muted">管理员可以指定哪些普通用户可以查看和预览这个项目。</div>
        </div>
        <button type="button" class="secondary" id="open-permission-editor-btn">编辑权限</button>
      </div>
      <div class="muted">${escapeHtml(permissionMembersSummaryView(project))}</div>
    </section>
  `;
}

function versionActionsView(project, version) {
  return `
    <div class="row" style="justify-content:flex-end;">
      ${version.status === "ready" ? `<a href="/preview/${version.id}" target="_blank"><button class="ghost" ${busyDisabledAttr()}>预览</button></a>` : ""}
      ${canManageProjects() && version.status === "ready" && project.currentVersionId !== version.id ? `<button class="switch-version-btn" data-version-id="${version.id}" ${busyDisabledAttr()}>设为当前版本</button>` : ""}
      ${canManageProjects() ? `<button class="danger delete-version-btn" data-version-id="${version.id}" data-version-no="${version.versionNo}" ${busyDisabledAttr()}>${state.busy ? "删除中..." : "删除版本"}</button>` : ""}
    </div>
  `;
}

function versionEditorActionsView(version) {
  if (!canManageProjects()) return "";
  return `
    <div class="row" style="margin-top:12px;">
      <button class="secondary edit-version-label-btn" data-version-id="${version.id}">编辑标签</button>
      ${version.status === "ready" ? `<button class="secondary edit-entry-file-btn" data-version-id="${version.id}">编辑入口页</button>` : ""}
    </div>
  `;
}

function versionEditorModalView() {
  if (!state.versionEditor) return "";
  const { type, version } = state.versionEditor;
  const isEntryEditor = type === "entry";

  return `
    <div class="modal-backdrop">
      <section class="modal-card">
        <div class="section-title">
          <div>
            <h3 style="margin:0;">${isEntryEditor ? "编辑入口页" : "编辑版本标签"}</h3>
            <div class="muted">v${version.versionNo}${version.label ? ` / ${escapeHtml(version.label)}` : ""}</div>
          </div>
          <button type="button" class="ghost close-version-editor-btn">关闭</button>
        </div>
        ${isEntryEditor ? `
          <div>
            <label>入口页</label>
            <select id="version-editor-entry-file">
              ${(version.htmlFiles || []).map(file => `<option value="${escapeHtml(file)}" ${file === version.entryFile ? "selected" : ""}>${escapeHtml(file)}</option>`).join("")}
            </select>
          </div>
          <div class="row" style="margin-top:16px; justify-content:flex-end;">
            <button type="button" class="secondary close-version-editor-btn">取消</button>
            <button type="button" id="save-version-entry-btn" data-version-id="${version.id}">保存入口页</button>
          </div>
        ` : `
          <div>
            <label>版本标签</label>
            <input id="version-editor-label" maxlength="120" placeholder="例如：评审版、提测版、演示版" value="${escapeHtml(version.label || "")}" />
          </div>
          <div class="row" style="margin-top:16px; justify-content:flex-end;">
            <button type="button" class="secondary close-version-editor-btn">取消</button>
            <button type="button" id="save-version-label-btn" data-version-id="${version.id}">保存标签</button>
          </div>
        `}
      </section>
    </div>
  `;
}

function permissionEditorModalView() {
  if (!state.permissionEditor || !state.currentProject) return "";
  const { available, selected, availablePicked, selectedPicked } = state.permissionEditor;

  return `
    <div class="modal-backdrop">
      <section class="modal-card modal-card-wide">
        <div class="section-title">
          <div>
            <h3 style="margin:0;">编辑项目权限</h3>
            <div class="muted">${escapeHtml(state.currentProject.name)}</div>
          </div>
          <button type="button" class="ghost close-permission-editor-btn">关闭</button>
        </div>
        <div class="transfer-layout">
          <div class="transfer-panel">
            <div class="transfer-title">待授权用户</div>
            <select id="permission-available-select" multiple size="10">
              ${available.map(user => `<option value="${user.id}" ${availablePicked.includes(user.id) ? "selected" : ""}>${escapeHtml(user.displayName)} (${escapeHtml(user.username)})</option>`).join("")}
            </select>
          </div>
          <div class="transfer-actions">
            <button type="button" class="secondary move-to-selected-btn">&gt;</button>
            <button type="button" class="secondary move-to-available-btn">&lt;</button>
          </div>
          <div class="transfer-panel">
            <div class="transfer-title">已授权用户</div>
            <select id="permission-selected-select" multiple size="10">
              ${selected.map(user => `<option value="${user.id}" ${selectedPicked.includes(user.id) ? "selected" : ""}>${escapeHtml(user.displayName)} (${escapeHtml(user.username)})</option>`).join("")}
            </select>
          </div>
        </div>
        <div class="row" style="margin-top:16px; justify-content:flex-end;">
          <button type="button" class="secondary close-permission-editor-btn">取消</button>
          <button type="button" id="save-project-members-btn">保存权限</button>
        </div>
      </section>
    </div>
  `;
}

function projectEditorModalView() {
  if (!state.projectEditor || !state.currentProject) return "";
  return `
    <div class="modal-backdrop">
      <section class="modal-card">
        <div class="section-title">
          <div>
            <h3 style="margin:0;">编辑项目信息</h3>
            <div class="muted">${escapeHtml(state.currentProject.slug || "")}</div>
          </div>
          <button type="button" class="ghost close-project-editor-btn">关闭</button>
        </div>
        <div>
          <label>项目名称</label>
          <input id="project-editor-name" maxlength="160" value="${escapeHtml(state.projectEditor.name)}" />
        </div>
        <div style="margin-top:12px;">
          <label>项目描述</label>
          <textarea id="project-editor-description">${escapeHtml(state.projectEditor.description)}</textarea>
        </div>
        <div style="margin-top:12px;">
          <label>项目状态</label>
          <select id="project-editor-status">
            <option value="active" ${state.projectEditor.status === "active" ? "selected" : ""}>active</option>
            <option value="archived" ${state.projectEditor.status === "archived" ? "selected" : ""}>archived</option>
          </select>
        </div>
        <div class="row" style="margin-top:16px; justify-content:flex-end;">
          <button type="button" class="secondary close-project-editor-btn">取消</button>
          <button type="button" id="save-project-editor-btn">保存项目信息</button>
        </div>
      </section>
    </div>
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
          <button class="secondary" id="back-dashboard-btn">返回列表</button>
          ${project.currentVersionId ? `<a href="/preview/${project.currentVersionId}" target="_blank"><button>打开当前预览</button></a>` : ""}
          ${project.slug ? `<a href="/p/${project.slug}" target="_blank"><button class="ghost">快捷访问</button></a>` : ""}
        </div>
      </div>

      <div class="row">
        <div class="card" style="padding:16px;">
          <div class="section-title" style="margin-bottom:10px;">
            <h3 style="margin:0;">项目信息</h3>
            ${canManageProjects() ? `<button type="button" class="secondary" id="open-project-editor-btn" ${busyDisabledAttr()}>编辑</button>` : ""}
          </div>
          <div>Slug: <strong>${escapeHtml(project.slug)}</strong></div>
          <div class="muted">名称: ${escapeHtml(project.name)}</div>
          <div class="muted">Owner: ${escapeHtml(project.owner?.displayName || "-")}</div>
          <div class="muted">状态: ${escapeHtml(project.status || "-")}</div>
          <div class="muted">当前版本: ${project.currentVersion ? `v${project.currentVersion.versionNo}` : "未设置"}</div>
          <div class="muted">已授权用户: ${(project.members || []).length}</div>
          <div class="muted">描述: ${escapeHtml(project.description || "暂无描述")}</div>
        </div>

        <div class="card" style="padding:16px;">
          <h3 style="margin-top:0;">版本发布</h3>
          ${canManageProjects() ? `
            <form id="upload-version-form">
              <label>ZIP 文件</label>
              <input type="file" name="file" accept=".zip" required />
              <button style="margin-top:12px;">上传并发布</button>
            </form>
          ` : `<div class="muted">当前账号没有上传版本的权限。</div>`}
        </div>
      </div>
    </section>

    ${projectPermissionsView(project)}

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
                ${version.label ? `<span class="tag version-tag">${escapeHtml(version.label)}</span>` : ""}
              </div>
              ${versionActionsView(project, version)}
            </div>
            <div class="muted">入口文件: ${escapeHtml(version.entryFile || "-")}</div>
            <div class="muted">版本标签: ${escapeHtml(version.label || "-")}</div>
            ${versionEditorActionsView(version)}
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
            <label>系统角色</label>
            <select name="roles" multiple size="3">
              <option value="admin">admin</option>
              <option value="project_admin">project_admin</option>
              <option value="viewer" selected>viewer</option>
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
                  ${["admin", "project_admin", "viewer"].map(code => `<option value="${code}" ${(user.roles || []).some(role => role.code === code) ? "selected" : ""}>${code}</option>`).join("")}
                </select>
              </td>
              <td>
                <div class="row">
                  <button class="secondary save-roles-btn" data-user-id="${user.id}">保存角色</button>
                  <button class="secondary reset-password-btn" data-user-id="${user.id}">重置密码</button>
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
  const totalPages = Math.max(1, Math.ceil((state.auditPagination.total || 0) / state.auditPagination.pageSize));
  return `
    <section class="card">
      <div class="section-title">
        <div>
          <h2 style="margin:0;">审计日志</h2>
          <div class="muted">记录登录、用户管理、项目授权、入口页调整和版本发布等操作。</div>
        </div>
        <div class="row" style="justify-content:flex-end; align-items:end;">
          <div style="flex:0 0 120px;">
            <label>每页条数</label>
            <select id="audit-page-size">
              ${[10, 20, 50, 100].map(size => `<option value="${size}" ${state.auditPagination.pageSize === size ? "selected" : ""}>${size}</option>`).join("")}
            </select>
          </div>
          <button id="reload-audit-btn" class="secondary">刷新</button>
        </div>
      </div>
      <div class="muted" style="margin-bottom:12px;">共 ${state.auditPagination.total} 条，当前第 ${state.auditPagination.page} / ${totalPages} 页。</div>
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
      <div class="pagination-bar">
        <button id="audit-prev-btn" class="secondary" ${state.auditPagination.page <= 1 ? "disabled" : ""}>上一页</button>
        <span class="muted">第 ${state.auditPagination.page} 页 / 共 ${totalPages} 页</span>
        <button id="audit-next-btn" class="secondary" ${state.auditPagination.page >= totalPages ? "disabled" : ""}>下一页</button>
      </div>
    </section>
  `;
}

function mainView() {
  return `
    <div class="shell">
      <div class="hero">
        <div>
          <h1>Prototype Hub</h1>
          <p>一站式原型仓库。</p>
        </div>
        <div>
          <strong>${escapeHtml(state.me.displayName)}</strong>
          <div class="muted">${escapeHtml(state.me.username)} / ${state.me.roles.join(", ")}</div>
        </div>
      </div>

      <div class="layout">
        <aside class="sidebar">
          <button class="${navButtonClass("dashboard")}" data-route="dashboard">项目工作台</button>
          ${canManageUsers() ? `<button class="${navButtonClass("users", "secondary")}" data-route="users">用户管理</button>` : ""}
          ${canViewAudit() ? `<button class="${navButtonClass("audit", "secondary")}" data-route="audit">审计日志</button>` : ""}
          <button class="ghost" id="logout-btn">退出登录</button>
          <details class="sidebar-panel sidebar-details" id="change-password-panel">
            <summary class="sidebar-panel-title">修改密码</summary>
            <form id="change-password-form">
              <label>当前密码</label>
              <input name="currentPassword" type="password" required />
              <label style="margin-top:10px;">新密码</label>
              <input name="newPassword" type="password" minlength="8" required />
              <button type="submit" class="secondary" style="margin-top:12px;">保存密码</button>
            </form>
          </details>
          ${state.error ? `<div class="message error">${escapeHtml(state.error)}</div>` : ""}
          ${state.message ? `<div class="message">${escapeHtml(state.message)}</div>` : ""}
        </aside>

        <main class="content">
          ${state.route === "dashboard" ? dashboardView() : ""}
          ${state.route === "project" ? projectView() : ""}
          ${state.route === "users" && canManageUsers() ? usersView() : ""}
          ${state.route === "audit" ? auditView() : ""}
        </main>
      </div>
      ${versionEditorModalView()}
      ${permissionEditorModalView()}
      ${projectEditorModalView()}
      ${state.busy ? `
        <div class="busy-backdrop">
          <div class="busy-panel">
            <div class="busy-spinner"></div>
            <div>${escapeHtml(state.busy)}</div>
            <div class="muted">删除尚未完成，请稍候，不要重复操作。</div>
          </div>
        </div>
      ` : ""}
    </div>
  `;
}

function render() {
  const app = document.getElementById("app");
  app.innerHTML = state.me ? mainView() : loginView();
  if (state.me) {
    bindShell();
  } else {
    bindLogin();
  }
}

function bindLogin() {
  document.getElementById("login-form").addEventListener("submit", async event => {
    event.preventDefault();
    const form = new FormData(event.target);

    try {
      setMessage("正在登录...");
      await api("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({
          username: form.get("username"),
          password: form.get("password"),
        }),
      });
      await bootstrap();
      setMessage("登录成功");
    } catch (error) {
      setMessage(error.message, true);
    }
  });
}

function bindProjectFilters() {
  const keywordInput = document.getElementById("project-filter-keyword");
  if (keywordInput) {
    keywordInput.addEventListener("input", event => {
      state.projectFilters.keyword = event.target.value;
      render();
    });
  }

  const statusSelect = document.getElementById("project-filter-status");
  if (statusSelect) {
    statusSelect.addEventListener("change", event => {
      state.projectFilters.status = event.target.value;
      render();
    });
  }

  const releaseSelect = document.getElementById("project-filter-release");
  if (releaseSelect) {
    releaseSelect.addEventListener("change", event => {
      state.projectFilters.release = event.target.value;
      render();
    });
  }
}

function bindShell() {
  document.querySelectorAll("[data-route]").forEach(button => {
    button.addEventListener("click", async () => {
      if (state.busy) return;
      state.route = button.dataset.route;
      if (state.route === "users" && canManageUsers()) await loadUsers();
      if (state.route === "audit" && canViewAudit()) await loadAudit();
      render();
    });
  });

  bindProjectFilters();

  const logoutBtn = document.getElementById("logout-btn");
  if (logoutBtn) {
    logoutBtn.addEventListener("click", async () => {
      if (state.busy) return;
      await api("/api/auth/logout", { method: "POST" });
      Object.assign(state, { me: null, projects: [], currentProject: null, route: "dashboard" });
      render();
    });
  }

  const changePasswordForm = document.getElementById("change-password-form");
  if (changePasswordForm) {
    changePasswordForm.addEventListener("submit", async event => {
      event.preventDefault();
      if (state.busy) return;
      const form = new FormData(event.target);
      try {
        await api("/api/me/password", {
          method: "POST",
          body: JSON.stringify({
            currentPassword: form.get("currentPassword"),
            newPassword: form.get("newPassword"),
          }),
        });
        event.target.reset();
        setMessage("密码已更新");
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  }

  const createProjectForm = document.getElementById("create-project-form");
  if (createProjectForm) {
    createProjectForm.addEventListener("submit", async event => {
      event.preventDefault();
      if (state.busy) return;
      const form = new FormData(event.target);
      try {
        await api("/api/projects", {
          method: "POST",
          body: JSON.stringify({
            name: form.get("name"),
            description: form.get("description"),
          }),
        });
        event.target.reset();
        setMessage("项目已创建");
        await loadProjects();
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  }

  const reloadProjectsBtn = document.getElementById("reload-projects-btn");
  if (reloadProjectsBtn) {
    reloadProjectsBtn.addEventListener("click", () => {
      if (state.busy) return;
      loadProjects();
    });
  }

  document.querySelectorAll(".open-project-btn").forEach(button => {
    button.addEventListener("click", async () => {
      if (state.busy) return;
      try {
        await openProject(button.dataset.projectId);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  document.querySelectorAll(".delete-project-btn").forEach(button => {
    button.addEventListener("click", async () => {
      if (state.busy) return;
      const projectName = button.dataset.projectName || "该项目";
      if (!window.confirm(`确认删除项目“${projectName}”吗？项目版本记录也会一并删除。`)) {
        return;
      }
      try {
        beginBusy(`正在删除项目“${projectName}”...`);
        await api(`/api/projects/${button.dataset.projectId}`, { method: "DELETE" });
        if (state.currentProject?.id === Number(button.dataset.projectId)) {
          state.currentProject = null;
          state.route = "dashboard";
        }
        setMessage("项目已删除");
        await loadProjects();
      } catch (error) {
        setMessage(error.message, true);
      } finally {
        endBusy();
      }
    });
  });

  const backBtn = document.getElementById("back-dashboard-btn");
  if (backBtn) {
    backBtn.addEventListener("click", async () => {
      if (state.busy) return;
      state.route = "dashboard";
      state.currentProject = null;
      await loadProjects();
      render();
    });
  }

  const uploadForm = document.getElementById("upload-version-form");
  if (uploadForm) {
    uploadForm.addEventListener("submit", async event => {
      event.preventDefault();
      if (state.busy) return;
      const formData = new FormData(event.target);
      try {
        setMessage("ZIP 已提交，后台正在处理...");
        const response = await fetch(`/api/projects/${state.currentProject.id}/versions/upload`, {
          method: "POST",
          credentials: "include",
          body: formData,
        });
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
  }

  document.querySelectorAll(".switch-version-btn").forEach(button => {
    button.addEventListener("click", async () => {
      if (state.busy) return;
      try {
        const data = await api(`/api/projects/${state.currentProject.id}/current-version`, {
          method: "POST",
          body: JSON.stringify({ versionId: Number(button.dataset.versionId) }),
        });
        if (data.project) {
          state.currentProject = data.project;
          upsertProject(data.project);
        }
        setMessage("当前版本已切换");
        render();
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  document.querySelectorAll(".delete-version-btn").forEach(button => {
    button.addEventListener("click", async () => {
      if (state.busy) return;
      const versionNo = button.dataset.versionNo || "";
      if (!window.confirm(`确认删除 v${versionNo} 吗？原始 ZIP 和预览文件也会一起删除。`)) {
        return;
      }
      try {
        beginBusy(`正在删除版本 v${versionNo}...`);
        const data = await api(`/api/projects/${state.currentProject.id}/versions/${button.dataset.versionId}`, {
          method: "DELETE",
        });
        if (data.project) {
          state.currentProject = data.project;
          upsertProject(data.project);
        } else {
          await openProject(state.currentProject.id);
        }
        state.versionEditor = null;
        setMessage("版本已删除");
        render();
      } catch (error) {
        setMessage(error.message, true);
      } finally {
        endBusy();
      }
    });
  });

  document.querySelectorAll(".edit-entry-file-btn").forEach(button => {
    button.addEventListener("click", async () => {
      try {
        await openVersionEditor("entry", button.dataset.versionId);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  document.querySelectorAll(".edit-version-label-btn").forEach(button => {
    button.addEventListener("click", async () => {
      try {
        await openVersionEditor("label", button.dataset.versionId);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  });

  document.querySelectorAll(".close-version-editor-btn").forEach(button => {
    button.addEventListener("click", closeVersionEditor);
  });

  const openProjectEditorBtn = document.getElementById("open-project-editor-btn");
  if (openProjectEditorBtn) {
    openProjectEditorBtn.addEventListener("click", () => {
      if (state.busy) return;
      openProjectEditor();
    });
  }

  document.querySelectorAll(".close-project-editor-btn").forEach(button => {
    button.addEventListener("click", closeProjectEditor);
  });

  const openPermissionEditorBtn = document.getElementById("open-permission-editor-btn");
  if (openPermissionEditorBtn) {
    openPermissionEditorBtn.addEventListener("click", openPermissionEditor);
  }

  document.querySelectorAll(".close-permission-editor-btn").forEach(button => {
    button.addEventListener("click", closePermissionEditor);
  });

  const permissionAvailableSelect = document.getElementById("permission-available-select");
  if (permissionAvailableSelect) {
    permissionAvailableSelect.addEventListener("change", event => {
      updatePermissionSelection("availablePicked", Array.from(event.target.selectedOptions).map(option => option.value));
    });
  }

  const permissionSelectedSelect = document.getElementById("permission-selected-select");
  if (permissionSelectedSelect) {
    permissionSelectedSelect.addEventListener("change", event => {
      updatePermissionSelection("selectedPicked", Array.from(event.target.selectedOptions).map(option => option.value));
    });
  }

  document.querySelectorAll(".move-to-selected-btn").forEach(button => {
    button.addEventListener("click", () => movePermissionItems("available", "selected", "availablePicked"));
  });

  document.querySelectorAll(".move-to-available-btn").forEach(button => {
    button.addEventListener("click", () => movePermissionItems("selected", "available", "selectedPicked"));
  });

  const saveVersionEntryBtn = document.getElementById("save-version-entry-btn");
  if (saveVersionEntryBtn) {
    saveVersionEntryBtn.addEventListener("click", async () => {
      const select = document.getElementById("version-editor-entry-file");
      try {
        await api(`/api/projects/${state.currentProject.id}/versions/${saveVersionEntryBtn.dataset.versionId}/entry-file`, {
          method: "POST",
          body: JSON.stringify({ entryFile: select.value }),
        });
        state.versionEditor = null;
        setMessage("入口页已更新");
        await openProject(state.currentProject.id);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  }

  const saveVersionLabelBtn = document.getElementById("save-version-label-btn");
  if (saveVersionLabelBtn) {
    saveVersionLabelBtn.addEventListener("click", async () => {
      const input = document.getElementById("version-editor-label");
      try {
        await api(`/api/projects/${state.currentProject.id}/versions/${saveVersionLabelBtn.dataset.versionId}/label`, {
          method: "POST",
          body: JSON.stringify({ label: input.value.trim() }),
        });
        state.versionEditor = null;
        setMessage("版本标签已更新");
        await openProject(state.currentProject.id);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  }

  const saveProjectMembersBtn = document.getElementById("save-project-members-btn");
  if (saveProjectMembersBtn) {
    saveProjectMembersBtn.addEventListener("click", async () => {
      const userIds = (state.permissionEditor?.selected || []).map(user => user.id);
      try {
        await api(`/api/admin/projects/${state.currentProject.id}/members`, {
          method: "POST",
          body: JSON.stringify({ userIds }),
        });
        state.permissionEditor = null;
        setMessage("项目权限已更新");
        await openProject(state.currentProject.id);
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  }

  const saveProjectEditorBtn = document.getElementById("save-project-editor-btn");
  if (saveProjectEditorBtn) {
    saveProjectEditorBtn.addEventListener("click", async () => {
      if (state.busy || !state.currentProject) return;
      const nameInput = document.getElementById("project-editor-name");
      const descriptionInput = document.getElementById("project-editor-description");
      const statusInput = document.getElementById("project-editor-status");
      try {
        const data = await api(`/api/projects/${state.currentProject.id}`, {
          method: "PATCH",
          body: JSON.stringify({
            name: nameInput.value.trim(),
            description: descriptionInput.value.trim(),
            status: statusInput.value,
          }),
        });
        state.projectEditor = null;
        if (data.project) {
          state.currentProject = data.project;
          upsertProject(data.project);
        } else {
          await openProject(state.currentProject.id);
        }
        setMessage("项目信息已更新");
        render();
      } catch (error) {
        setMessage(error.message, true);
      }
    });
  }

  const createUserForm = document.getElementById("create-user-form");
  if (createUserForm) {
    createUserForm.addEventListener("submit", async event => {
      event.preventDefault();
      const form = new FormData(event.target);
      const select = event.target.querySelector('select[name="roles"]');
      const roles = Array.from(select.selectedOptions).map(option => option.value);
      if (!roles.length) roles.push("viewer");
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
  }

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

  document.querySelectorAll(".reset-password-btn").forEach(button => {
    button.addEventListener("click", async () => {
      const password = window.prompt("请输入新密码");
      if (!password) return;
      try {
        await api(`/api/admin/users/${button.dataset.userId}/reset-password`, {
          method: "POST",
          body: JSON.stringify({ password }),
        });
        setMessage("密码已重置");
      } catch (error) {
        setMessage(error.message, true);
      }
    });
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

  const reloadAuditBtn = document.getElementById("reload-audit-btn");
  if (reloadAuditBtn) {
    reloadAuditBtn.addEventListener("click", () => loadAudit(state.auditPagination.page));
  }

  const auditPageSize = document.getElementById("audit-page-size");
  if (auditPageSize) {
    auditPageSize.addEventListener("change", async event => {
      state.auditPagination.pageSize = Number(event.target.value);
      state.auditPagination.page = 1;
      await loadAudit(1);
    });
  }

  const auditPrevBtn = document.getElementById("audit-prev-btn");
  if (auditPrevBtn) {
    auditPrevBtn.addEventListener("click", async () => {
      if (state.auditPagination.page <= 1) return;
      await loadAudit(state.auditPagination.page - 1);
    });
  }

  const auditNextBtn = document.getElementById("audit-next-btn");
  if (auditNextBtn) {
    auditNextBtn.addEventListener("click", async () => {
      const totalPages = Math.max(1, Math.ceil((state.auditPagination.total || 0) / state.auditPagination.pageSize));
      if (state.auditPagination.page >= totalPages) return;
      await loadAudit(state.auditPagination.page + 1);
    });
  }
}

async function bootstrap() {
  const me = await api("/api/me");
  state.me = me.user;
  state.route = "dashboard";
  await loadProjects();
  if (canManageProjects()) {
    await loadMemberCandidates();
  }
  if (canManageUsers()) {
    await loadUsers();
  }
  if (canViewAudit()) {
    await loadAudit();
  }
  render();
}

async function loadProjects() {
  const data = await api("/api/projects");
  state.projects = data.items || [];
  if (state.currentProject) {
    const matched = state.projects.find(project => project.id === state.currentProject.id);
    if (matched) {
      state.currentProject = { ...state.currentProject, ...matched };
    }
  }
  if (state.route === "dashboard") render();
}

async function openProject(projectId) {
  if (canManageProjects() && state.memberCandidates.length === 0) await loadMemberCandidates();
  const data = await api(`/api/projects/${projectId}`);
  state.versionEditor = null;
  state.projectEditor = null;
  state.permissionEditor = null;
  state.currentProject = data.project;
  upsertProject(data.project);
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
      if (latest?.status === "ready") {
        setMessage("版本处理完成");
      } else {
        setMessage(`版本处理失败：${latest?.errorMessage || "unknown error"}`, true);
      }
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

async function loadMemberCandidates() {
  const data = await api("/api/users");
  state.memberCandidates = data.items || [];
}

async function loadAudit(page = state.auditPagination.page) {
  const params = new URLSearchParams({
    page: String(page),
    pageSize: String(state.auditPagination.pageSize),
  });
  const data = await api(`/api/admin/audit-logs?${params.toString()}`);
  state.auditLogs = data.items || [];
  state.auditPagination.page = data.page || page;
  state.auditPagination.pageSize = data.pageSize || state.auditPagination.pageSize;
  state.auditPagination.total = data.total || 0;
  if (state.route === "audit") render();
}

(async function init() {
  try {
    await bootstrap();
  } catch (_) {
    render();
  }
})();
