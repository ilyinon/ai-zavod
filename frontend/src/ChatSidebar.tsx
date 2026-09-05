import { FormEvent, useState } from 'react';
import { Archive, Check, ChevronDown, ChevronRight, Folder, MessageSquare, MoreHorizontal, Pencil, Pin, Plus, Search, Settings, Trash2, X } from 'lucide-react';
import { Project, Task } from './lib/backend';

type Props = {
  projects: Project[]; chats: Task[]; selectedId: string; busy: Record<string, string>;
  onNew: (projectId?: string) => void; onSelect: (id: string) => void;
  onUpdate: (task: Task) => Promise<void>; onDelete: (id: string) => Promise<void>;
  onProject: () => void; onSettings: () => void;
  onEditProject: (project: Project) => void;
};

export function ChatSidebar(props: Props) {
  const [query, setQuery] = useState('');
  const [archive, setArchive] = useState(false);
  const [collapsed, setCollapsed] = useState<string[]>([]);
  const [editing, setEditing] = useState<Task | null>(null);
  const [deleting, setDeleting] = useState<Task | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const visible = props.chats.filter(t => (t.status === 'archived') === archive && t.title.toLocaleLowerCase().includes(query.toLocaleLowerCase()));
  async function save(event: FormEvent) {
    event.preventDefault(); if (!editing) return;
    setSaving(true); setError('');
    try { await props.onUpdate(editing); setEditing(null); } catch (err) { setError(String(err)); } finally { setSaving(false); }
  }
  function row(task: Task) {
    const busy = props.busy[task.id] && props.busy[task.id] !== 'idle';
    return <div key={task.id} className={`chat-nav-row ${props.selectedId === task.id ? 'selected' : ''}`}>
      <button type="button" className="chat-nav-select" onClick={() => props.onSelect(task.id)} title={task.title}>
        {busy ? <span className="chat-busy-dot" /> : task.pinned ? <Pin size={14} /> : <MessageSquare size={14} />}
        <span>{task.title}</span>
      </button>
      <details className="chat-menu">
        <summary aria-label={`Действия: ${task.title}`} title="Действия"><MoreHorizontal size={16} /></summary>
        <div className="chat-menu-items">
          <button disabled={!!busy} onClick={() => { setError(''); setEditing({ ...task }); }}><Pencil size={14} />Изменить</button>
          <button disabled={!!busy} onClick={() => void props.onUpdate({ ...task, pinned: !task.pinned }).catch(err => setError(String(err)))}><Pin size={14} />{task.pinned ? 'Открепить' : 'Закрепить'}</button>
          <button disabled={!!busy} onClick={() => void props.onUpdate({ ...task, status: archive ? 'active' : 'archived' }).catch(err => setError(String(err)))}><Archive size={14} />{archive ? 'Восстановить' : 'В архив'}</button>
          <button disabled={!!busy} onClick={() => setDeleting(task)}><Trash2 size={14} />Удалить чат</button>
        </div>
      </details>
    </div>;
  }
  return <aside className="sidebar projects-panel chat-navigation">
    <div className="chat-nav-heading"><span>Zavod AI</span><button type="button" className="icon-button" title="Новый чат" aria-label="Новый чат" onClick={() => props.onNew()}><Plus size={19} /></button></div>
    <button className="new-chat-command" onClick={() => props.onNew()}><Plus size={17} />Новый чат</button>
    <label className="chat-search"><Search size={16} /><input aria-label="Поиск чатов и проектов" placeholder="Поиск" value={query} onChange={e => setQuery(e.target.value)} /></label>
    <div className="chat-nav-scroll">
      <div className="chat-section-label">{archive ? 'Архив' : 'Чаты'}</div>
      {visible.filter(t => !t.projectId).map(row)}
      <div className="chat-section-label"><span>Проекты</span><button className="project-icon-button" title="Добавить проект" aria-label="Добавить проект" onClick={props.onProject}><Plus size={16} /></button></div>
      {props.projects.map(project => {
        const tasks = visible.filter(t => t.projectId === project.id);
        if (query && !tasks.length && !project.name.toLocaleLowerCase().includes(query.toLocaleLowerCase())) return null;
        const closed = collapsed.includes(project.id) && !query;
        return <div key={project.id} className="chat-project-group">
          <div className="chat-project-heading">
            <button className="chat-nav-select" title={project.path} onClick={() => setCollapsed(ids => closed ? ids.filter(id => id !== project.id) : [...ids, project.id])}>{closed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}<Folder size={15} /><span>{project.name}</span></button>
            <button className="project-icon-button" aria-label={`Новый чат в ${project.name}`} title="Новый чат в проекте" onClick={() => props.onNew(project.id)}><Plus size={15} /></button>
            <button className="project-icon-button" aria-label={`Настройки ${project.name}`} title="Настройки проекта" onClick={() => props.onEditProject(project)}><MoreHorizontal size={15} /></button>
          </div>
          {!closed && <div className="project-chats">{tasks.map(row)}</div>}
        </div>;
      })}
      {!visible.length && <p className="muted chat-nav-empty">{query ? 'Ничего не найдено' : archive ? 'Архив пуст' : 'Нет чатов'}</p>}
    </div>
    {error && !editing && <p className="error-banner">{error}</p>}
    <div className="chat-nav-footer"><button onClick={() => setArchive(!archive)}><Archive size={16} />{archive ? 'Все чаты' : 'Архив'}</button><button title="Настройки" aria-label="Настройки" onClick={props.onSettings}><Settings size={18} /></button></div>
    {editing && <div className="chat-modal-backdrop" onClick={() => !saving && setEditing(null)}><form className="chat-dialog" onSubmit={save} onClick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Изменить чат">
      <div className="chat-nav-heading"><h3>Изменить чат</h3><button type="button" className="icon-button" aria-label="Закрыть" onClick={() => setEditing(null)}><X size={18} /></button></div>
      <label>Название<input className="input" autoFocus value={editing.title} onChange={e => setEditing({ ...editing, title: e.target.value })} /></label>
      <label>Проект<select className="input" value={editing.projectId} onChange={e => setEditing({ ...editing, projectId: e.target.value })}><option value="">Без проекта</option>{props.projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
      {error && <p className="error-banner">{error}</p>}<button type="submit" disabled={saving || !editing.title.trim()}><Check size={16} />Сохранить</button>
    </form></div>}
    {deleting && <div className="chat-modal-backdrop"><div className="chat-dialog" role="alertdialog" aria-modal="true" aria-label="Удалить чат"><h3>Удалить «{deleting.title}»?</h3><p>Переписка будет удалена. Файлы проекта останутся на диске.</p>{error && <p className="error-banner">{error}</p>}<div className="chat-dialog-actions"><button disabled={saving} onClick={async () => { setSaving(true); try { await props.onDelete(deleting.id); setDeleting(null); } catch (err) { setError(String(err)); } finally { setSaving(false); } }}><Trash2 size={16} />Удалить</button><button disabled={saving} onClick={() => setDeleting(null)}>Отмена</button></div></div></div>}
  </aside>;
}
