import { Routes, Route } from 'react-router-dom'
import Lobby from './pages/Lobby'
import Room from './pages/Room'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Lobby />} />
      <Route path="/room/:matchId" element={<Room />} />
    </Routes>
  )
}
