import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

type SessionStatus = 'connecting' | 'ready' | 'running' | 'stopping' | 'offline'
type ItemKind = 'user' | 'assistant' | 'thinking' | 'tool' | 'approval' | 'system' | 'error'

type ServerEvent = {
  type?: string
  content?: string
  result?: string
  error?: string
  tool_name?: string
  request_id?: string
  decision?: string
  risk?: string
  reason?: string
  is_error?: boolean
}

type ChatItem = {
  id: string
  kind: ItemKind
  content: string
  title?: string
  requestId?: string
  risk?: string
  resolved?: string
}

type Session = {
  id: string
  status: SessionStatus
  items: ChatItem[]
  draft: string
  streamItemId?: string
  createdAt: number
}

const wsURL = () => {
  const configuredURL = import.meta.env.VITE_WS_URL as string | undefined
  if (configuredURL) return configuredURL
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/ws`
}

const statusText: Record<SessionStatus, string> = {
  connecting: '连接中',
  ready: '空闲',
  running: '运行中',
  stopping: '停止中',
  offline: '已断开',
}

const itemId = () => `${Date.now()}-${Math.random().toString(36).slice(2)}`

function App() {
  const sockets = useRef<Record<string, WebSocket>>({})
  const sequence = useRef(1)
  const [sessions, setSessions] = useState<Session[]>([])
  const [activeID, setActiveID] = useState('')

  const activeSession = useMemo(
    () => sessions.find((session) => session.id === activeID),
    [activeID, sessions],
  )

  const updateSession = useCallback((id: string, updater: (session: Session) => Session) => {
    setSessions((current) => current.map((session) => session.id === id ? updater(session) : session))
  }, [])

  const appendItem = useCallback((id: string, item: ChatItem) => {
    updateSession(id, (session) => ({ ...session, items: [...session.items, item] }))
  }, [updateSession])

  const handleEvent = useCallback((id: string, event: ServerEvent) => {
    switch (event.type) {
      case 'pong':
        updateSession(id, (session) => ({ ...session, status: 'ready' }))
        return
      case 'thinking':
        appendItem(id, { id: itemId(), kind: 'thinking', content: '模型正在思考…' })
        return
      case 'text_delta':
        updateSession(id, (session) => {
          if (session.streamItemId) {
            return {
              ...session,
              items: session.items.map((item) => item.id === session.streamItemId
                ? { ...item, content: item.content + (event.content ?? '') }
                : item),
            }
          }
          const id = itemId()
          return {
            ...session,
            streamItemId: id,
            items: [...session.items, { id, kind: 'assistant', content: event.content ?? '' }],
          }
        })
        return
      case 'text_completed':
        updateSession(id, (session) => {
          if (!event.content) return { ...session, streamItemId: undefined }
          const assistantID = itemId()
          return {
            ...session,
            streamItemId: undefined,
            items: [...session.items, { id: assistantID, kind: 'assistant', content: event.content }],
          }
        })
        return
      case 'tool_call':
        appendItem(id, {
          id: itemId(),
          kind: 'tool',
          title: `调用工具 · ${event.tool_name ?? 'unknown'}`,
          content: event.content ?? '',
        })
        return
      case 'tool_result':
        appendItem(id, {
          id: itemId(),
          kind: event.is_error ? 'error' : 'tool',
          title: `工具结果 · ${event.tool_name ?? 'unknown'}`,
          content: event.result ?? '',
        })
        return
      case 'approval_request':
        appendItem(id, {
          id: itemId(),
          kind: 'approval',
          title: `需要审批 · ${event.tool_name ?? 'unknown'}`,
          content: event.content ?? '',
          requestId: event.request_id,
          risk: event.risk,
        })
        updateSession(id, (session) => ({ ...session, status: 'running' }))
        return
      case 'task_completed':
        appendItem(id, { id: itemId(), kind: 'system', content: '任务完成' })
        updateSession(id, (session) => ({ ...session, status: 'ready', streamItemId: undefined }))
        return
      case 'task_canceled':
        appendItem(id, { id: itemId(), kind: 'system', content: '任务已取消' })
        updateSession(id, (session) => ({ ...session, status: 'ready', streamItemId: undefined }))
        return
      case 'task_failed':
      case 'error':
        appendItem(id, { id: itemId(), kind: 'error', content: event.error ?? '任务执行失败' })
        updateSession(id, (session) => ({ ...session, status: 'ready', streamItemId: undefined }))
        return
    }
  }, [appendItem, updateSession])

  const connectSession = useCallback((id: string) => {
    const socket = new WebSocket(wsURL())
    sockets.current[id] = socket
    updateSession(id, (session) => ({ ...session, status: 'connecting' }))

    socket.onopen = () => {
      updateSession(id, (session) => ({ ...session, status: 'ready' }))
      socket.send(JSON.stringify({ type: 'ping' }))
    }
    socket.onmessage = (message) => {
      try {
        handleEvent(id, JSON.parse(message.data) as ServerEvent)
      } catch {
        appendItem(id, { id: itemId(), kind: 'error', content: '服务端返回了无法解析的消息' })
      }
    }
    socket.onerror = () => {
      updateSession(id, (session) => ({ ...session, status: 'offline' }))
    }
    socket.onclose = () => {
      updateSession(id, (session) => ({ ...session, status: 'offline' }))
      delete sockets.current[id]
    }
  }, [appendItem, handleEvent, updateSession])

  const createSession = useCallback(() => {
    const id = `session-${sequence.current++}`
    setSessions((current) => [...current, {
      id,
      status: 'connecting',
      items: [],
      draft: '',
      createdAt: Date.now(),
    }])
    setActiveID(id)
    window.setTimeout(() => connectSession(id), 0)
  }, [connectSession])

  useEffect(() => {
    createSession()
    return () => Object.values(sockets.current).forEach((socket) => socket.close())
  }, [createSession])

  const send = useCallback((id: string, payload: object) => {
    const socket = sockets.current[id]
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      appendItem(id, { id: itemId(), kind: 'error', content: '当前 Session 尚未连接' })
      return false
    }
    socket.send(JSON.stringify(payload))
    return true
  }, [appendItem])

  const sendPrompt = useCallback((id: string) => {
    const session = sessions.find((item) => item.id === id)
    const content = session?.draft.trim()
    if (!content || session?.status === 'running' || session?.status === 'stopping') return
    if (!send(id, { type: 'prompt', content })) return
    appendItem(id, { id: itemId(), kind: 'user', content })
    updateSession(id, (current) => ({ ...current, draft: '', status: 'running' }))
  }, [appendItem, send, sessions, updateSession])

  const interrupt = useCallback((id: string) => {
    if (send(id, { type: 'interrupt' })) {
      updateSession(id, (session) => ({ ...session, status: 'stopping' }))
    }
  }, [send, updateSession])

  const respondApproval = useCallback((id: string, requestID: string | undefined, decision: string) => {
    if (!requestID || !send(id, { type: 'approval_response', request_id: requestID, decision })) return
    updateSession(id, (session) => ({
      ...session,
      items: session.items.map((item) => item.requestId === requestID ? { ...item, resolved: decision } : item),
    }))
  }, [send, updateSession])

  const closeSession = useCallback((id: string) => {
    const socket = sockets.current[id]
    if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'close' }))
    socket?.close()
    const next = sessions.filter((session) => session.id !== id)
    if (id === activeID) setActiveID(next[0]?.id ?? '')
    setSessions((current) => {
      return current.filter((session) => session.id !== id)
    })
  }, [activeID, sessions])

  const updateDraft = (value: string) => {
    if (!activeSession) return
    updateSession(activeSession.id, (session) => ({ ...session, draft: value }))
  }

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <div className="brand"><span className="brand-mark">✦</span><div><strong>Tiny Claw</strong><small>Agent Console</small></div></div>
        <button className="new-session" onClick={createSession}>＋ 新建 Session</button>
        <div className="session-heading"><span>并行会话</span><b>{sessions.length}</b></div>
        <div className="session-list">
          {sessions.map((session) => (
            <button className={`session-entry ${session.id === activeID ? 'active' : ''}`} key={session.id} onClick={() => setActiveID(session.id)}>
              <span className={`status-dot ${session.status}`} />
              <span className="session-copy"><strong>{session.id}</strong><small>{statusText[session.status]}</small></span>
              <span className="session-close" onClick={(event) => { event.stopPropagation(); closeSession(session.id) }}>×</span>
            </button>
          ))}
        </div>
        <div className="sidebar-foot"><span className="online-dot" />WebSocket · :8081</div>
      </aside>

      <section className="workspace">
        {activeSession ? (
          <>
            <header className="workspace-header">
              <div><span className="eyebrow">ACTIVE SESSION</span><h1>{activeSession.id}</h1></div>
              <div className="header-status"><span className={`status-dot ${activeSession.status}`} />{statusText[activeSession.status]}</div>
            </header>
            <div className="conversation">
              {activeSession.items.length === 0 && <div className="empty-state"><div className="empty-icon">✦</div><h2>开始一段 Agent 对话</h2><p>这个 Session 拥有独立上下文，可以和其他 Session 并行运行。</p></div>}
              {activeSession.items.map((item) => (
                <article className={`chat-item ${item.kind}`} key={item.id}>
                  <div className="item-label">{item.kind === 'user' ? 'YOU' : item.kind === 'assistant' ? 'AGENT' : item.kind === 'approval' ? 'APPROVAL' : item.kind.toUpperCase()}</div>
                  {item.title && <strong className="item-title">{item.title}{item.risk && <span className={`risk ${item.risk}`}>{item.risk}</span>}</strong>}
                  <pre>{item.content}</pre>
                  {item.kind === 'approval' && !item.resolved && <div className="approval-actions"><button onClick={() => respondApproval(activeSession.id, item.requestId, 'allow_once')}>允许一次</button><button onClick={() => respondApproval(activeSession.id, item.requestId, 'allow_session')}>会话允许</button><button className="danger" onClick={() => respondApproval(activeSession.id, item.requestId, 'deny')}>拒绝</button></div>}
                  {item.resolved && <span className="resolved">已选择：{item.resolved}</span>}
                </article>
              ))}
            </div>
            <footer className="composer-wrap">
              <textarea value={activeSession.draft} onChange={(event) => updateDraft(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') sendPrompt(activeSession.id) }} placeholder="输入消息，⌘ / Ctrl + Enter 发送" />
              <div className="composer-toolbar"><span>每个 Session 独立运行 · 支持实时流式输出</span>{activeSession.status === 'running' || activeSession.status === 'stopping' ? <button className="interrupt" onClick={() => interrupt(activeSession.id)}>■ 中断任务</button> : <button className="send" onClick={() => sendPrompt(activeSession.id)}>发送 ↗</button>}</div>
            </footer>
          </>
        ) : <div className="no-session"><h2>没有活动 Session</h2><button className="new-session" onClick={createSession}>＋ 新建 Session</button></div>}
      </section>
    </main>
  )
}

export default App
