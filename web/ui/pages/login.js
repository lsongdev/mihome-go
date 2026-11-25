import { html, useState, useAction, PageHeader, Alert } from '@lsongdev/scripts/react/index.js';
import { api, useApi, ApiNotice } from '../mi.js';

export default function Login() {
  const status = useApi('/login');
  const action = useAction();
  const [password, setPassword] = useState('');
  const [saved, setSaved] = useState(false);
  const submit = async event => {
    event.preventDefault();
    setSaved(false);
    const result = await action.run(() => api.post('/login', { password }));
    if (result?.saved) {
      setPassword('');
      setSaved(true);
      status.reload();
    }
  };
  const account = status.data;
  return html`<${PageHeader} title="小米账号登录" subtitle="重新生成并保存账号 token" />
    <${ApiNotice} error=${status.error} />
    <section class="card account-login stack">
      <p>登录成功后，token 会自动保存到服务端，供设备、音箱和摄像头使用。实时事件的独立授权请到<a href="#/events">事件页面</a>完成。</p>
      ${account ? html`<form class="stack" onSubmit=${submit}>
        <label class="stack">小米账号
          <input class="input" value=${account.username} readonly autocomplete="username" aria-label="小米账号" />
        </label>
        <label class="stack">密码
          <input class="input" type="password" autocomplete="current-password" value=${password}
            required=${!account.password_configured} disabled=${action.busy}
            placeholder=${account.password_configured ? '留空使用服务端已配置的密码' : '输入小米账号密码'}
            onInput=${event => setPassword(event.currentTarget.value)} />
        </label>
        <p class="muted">${account.saved ? '已有保存的 token；如登录过期，请重新登录。' : '尚未保存设备服务的 token。'} 输入的密码仅用于服务端当前进程，不写入配置文件。</p>
        <div class="button-row"><button class="button button-primary" disabled=${action.busy || !account.username}>
          ${action.busy ? '正在登录并保存…' : '登录并保存 token'}
        </button></div>
        ${!account.username ? html`<${Alert} variant="warning">请先在服务端配置 MI_USERNAME。<//>` : null}
      </form>` : null}
      <${ApiNotice} error=${action.error} />
      ${saved ? html`<${Alert} variant="success">登录成功，token 已保存。<a href="#/">返回设备列表</a><//>` : null}
    </section>`;
}
