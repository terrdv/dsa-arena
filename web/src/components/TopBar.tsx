import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { SwordsIcon } from './Icons'

export default function TopBar({ right }: { right?: ReactNode }) {
  return (
    <header className="topbar">
      <Link to="/" className="brand" aria-label="DSA Arena home">
        <SwordsIcon className="logo" />
        <span>
          DSA <em>ARENA</em>
        </span>
      </Link>
      {right}
    </header>
  )
}
