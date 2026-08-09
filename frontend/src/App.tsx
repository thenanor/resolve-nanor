import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { Nav } from './components/Nav'
import { TicketListPage } from './pages/TicketListPage'
import { TicketCreatePage } from './pages/TicketCreatePage'
import { TicketDetailPage } from './pages/TicketDetailPage'
import { AuditLogPage } from './pages/AuditLogPage'
import { StatsPage } from './pages/StatsPage'
import { CannedResponsesPage } from './pages/CannedResponsesPage'

function App() {
  return (
    <BrowserRouter>
      <Nav />
      <Routes>
        <Route path="/" element={<TicketListPage />} />
        <Route path="/tickets/new" element={<TicketCreatePage />} />
        <Route path="/tickets/:id" element={<TicketDetailPage />} />
        <Route path="/audit" element={<AuditLogPage />} />
        <Route path="/stats" element={<StatsPage />} />
        <Route path="/canned-responses" element={<CannedResponsesPage />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
