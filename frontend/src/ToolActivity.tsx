import { Check, CircleAlert, LoaderCircle, Terminal } from 'lucide-react';
import { useLayoutEffect, useRef, useState } from 'react';
import type { ToolInvocation } from './lib/backend';

const names: Record<string, string> = { list_files: 'Файлы проекта', read_file: 'Чтение файла', search_files: 'Поиск по коду', run_check: 'Проверка' };
const statuses: Record<string, string> = { running: 'В работе', passed: 'Готово', failed: 'Ошибка', blocked: 'Запрещено', cancelled: 'Отменено', interrupted: 'Прервано' };

function target(item: ToolInvocation) {
  try { const args = JSON.parse(item.arguments); return [args.command || args.query, args.path].filter(Boolean).join(' · '); }
  catch { return ''; }
}

export function ToolActivity({ items }: { items: ToolInvocation[] }) {
  const [open, setOpen] = useState(false);
  const anchor = useRef<HTMLElement>(null);
  const [position, setPosition] = useState({ left: 12, bottom: 12, maxHeight: 420 });
  useLayoutEffect(() => {
    if (!open) return;
    const update = () => {
      const box = anchor.current?.getBoundingClientRect();
      if (!box) return;
      const width = Math.min(560, window.innerWidth - 24);
      setPosition({ left: Math.max(12, Math.min(box.right - width, window.innerWidth - width - 12)), bottom: Math.max(12, window.innerHeight - box.top), maxHeight: Math.max(100, box.top - 16) });
    };
    update();
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    return () => { window.removeEventListener('resize', update); window.removeEventListener('scroll', update, true); };
  }, [open]);
  if (!items.length) return null;
  const running = items.some(item => item.result.status === 'running');
  return <section ref={anchor} className="tool-activity" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)} onKeyDown={event => { if (event.key === 'Escape') setOpen(false); }} onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false); }}>
    <button type="button" aria-expanded={open} onFocus={() => setOpen(true)} onClick={() => setOpen(true)}><Terminal size={16} /><span>{running ? 'Диагностика в работе' : `Инструменты · ${items.length}`}</span></button>
    {open && <section className="tool-activity-popover" style={position} aria-label="Вызовы инструментов">
      <header>Инструменты <span>Последние {items.length}</span></header>
      <div className="tool-activity-list">
        {items.map(item => <details key={item.id} className={`tool-invocation ${item.result.status}`}>
          <summary>
            {item.result.status === 'running' ? <LoaderCircle size={16} /> : item.result.status === 'passed' ? <Check size={16} /> : <CircleAlert size={16} />}
            <span><span>{names[item.tool] || item.tool}</span><small>{target(item)}</small></span>
            <small>{statuses[item.result.status] || item.result.status}</small>
          </summary>
          <div className="tool-invocation-output">
            <p>{item.agentName || item.agentId} · {new Date(item.startedAt).toLocaleTimeString()}{item.finishedAt ? ` · ${Math.max(0, (Date.parse(item.finishedAt) - Date.parse(item.startedAt)) / 1000).toFixed(1)} с` : ''}</p>
            {item.result.exitCode != null && <p>Код завершения: {item.result.exitCode}</p>}
            {item.result.error && <p role="status">{item.result.error}</p>}
            {item.result.output && <pre>{item.result.output}</pre>}
            {item.result.truncated && <p>Вывод сокращён по лимиту.</p>}
          </div>
        </details>)}
      </div>
    </section>}
  </section>;
}
