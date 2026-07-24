import { Fragment } from 'react'

type Block =
  | { kind: 'p'; text: string }
  | { kind: 'example'; label: string; lines: string[] }
  | { kind: 'constraints'; label: string; items: string[] }

const EXAMPLE_RE = /^Example\s*\d*\s*:?\s*$/
const LIST_SECTION_RE = /^(Constraints|Note|Follow-up)\s*:?\s*$/
const IO_RE = /^(Input|Output|Explanation)\s*:\s*/

/**
 * The dataset's problem_description is plain text with `Example N:` and
 * `Constraints:` sections. Parse it into styled blocks instead of dumping
 * one wall of text.
 */
function parse(description: string): Block[] {
  const blocks: Block[] = []
  let current: Block | null = null

  const flush = () => {
    if (current) blocks.push(current)
    current = null
  }

  for (const raw of description.split('\n')) {
    const line = raw.trim()

    if (EXAMPLE_RE.test(line)) {
      flush()
      current = { kind: 'example', label: line.replace(/:$/, ''), lines: [] }
      continue
    }
    if (LIST_SECTION_RE.test(line)) {
      flush()
      current = { kind: 'constraints', label: line.replace(/:$/, ''), items: [] }
      continue
    }

    if (!line) {
      // blank line ends a paragraph, but examples/constraints keep collecting
      if (current?.kind === 'p') flush()
      continue
    }

    if (current?.kind === 'example') {
      current.lines.push(line)
    } else if (current?.kind === 'constraints') {
      current.items.push(line)
    } else if (current?.kind === 'p') {
      current.text += ' ' + line
    } else {
      current = { kind: 'p', text: line }
    }
  }
  flush()

  return blocks
}

function ExampleLine({ line }: { line: string }) {
  const m = line.match(IO_RE)
  if (!m) return <div className="io-line">{line}</div>
  return (
    <div className="io-line">
      <span className="io-key">{m[1]}: </span>
      {line.slice(m[0].length)}
    </div>
  )
}

export default function ProblemBody({ description }: { description: string }) {
  const blocks = parse(description)

  return (
    <div className="problem-text">
      {blocks.map((block, i) => (
        <Fragment key={i}>
          {block.kind === 'p' && <p>{block.text}</p>}
          {block.kind === 'example' && (
            <>
              <h3 className="section-label">{block.label}</h3>
              <div className="example-block">
                {block.lines.map((line, j) => (
                  <ExampleLine key={j} line={line} />
                ))}
              </div>
            </>
          )}
          {block.kind === 'constraints' && (
            <>
              <h3 className="section-label">{block.label}</h3>
              <ul className="constraints">
                {block.items.map((item, j) => (
                  <li key={j}>
                    <code>{item}</code>
                  </li>
                ))}
              </ul>
            </>
          )}
        </Fragment>
      ))}
    </div>
  )
}
