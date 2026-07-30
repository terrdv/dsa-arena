import CodeMirror from '@uiw/react-codemirror'
import { python } from '@codemirror/lang-python'

export interface EditorProps {
  value: string
  language: string
  onChange: (value: string) => void
  readOnly?: boolean
}

// CodeMirror with the one-dark syntax palette; the editor chrome (background,
// gutter, selection) is restyled to the OLED tokens in index.css under
// `.editor-cm` so it sits flush inside a .panel.
export default function Editor({ value, language, onChange, readOnly }: EditorProps) {
  return (
    <CodeMirror
      className="editor-cm"
      value={value}
      onChange={onChange}
      readOnly={readOnly}
      editable={!readOnly}
      theme="dark"
      extensions={language === 'python' ? [python()] : []}
      basicSetup={{
        lineNumbers: true,
        foldGutter: false,
        highlightActiveLine: true,
        indentOnInput: true,
        bracketMatching: true,
        closeBrackets: true,
        autocompletion: false,
        tabSize: 4,
      }}
    />
  )
}
