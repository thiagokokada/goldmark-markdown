// Package markdown is a goldmark renderer that outputs markdown.
package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// NewNodeRenderer returns a new markdown Renderer that is configured by default values.
func NewNodeRenderer(options ...Option) renderer.NodeRenderer {
	r := &Renderer{
		Config: NewConfig(),
		writer: &defaultWriter{},
	}
	for _, opt := range options {
		opt.SetMarkdownOption(&r.Config)
	}
	return r
}

// NewRenderer returns a new renderer.Renderer containing a markdown NodeRenderer with defaults.
func NewRenderer(options ...Option) renderer.Renderer {
	r := NewNodeRenderer(options...)
	return renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(r, 1000)))
}

// Renderer is an implementation of renderer.Renderer that renders nodes as Markdown
type Renderer struct {
	Config
	writer Writer
}

// RegisterFuncs implements NodeRenderer.RegisterFuncs.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// blocks

	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	/* TODO
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)
	*/

	// inlines
	reg.Register(ast.KindText, r.renderText)
	/* TODO
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	*/
}

func (r *Renderer) renderDocument(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		// Add trailing newline to document if not already present
		b, l := r.writer.LastWriteBytes()
		if l == 0 || b[l-1] != byte('\n') {
			r.writer.Write(w, []byte("\n"))
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	// Empty headings or headings above level 2 can only be ATX
	if !n.HasChildren() || n.Level > 2 {
		return r.renderATXHeading(w, source, n, entering)
	}
	// Multiline headings can only be Setext
	if n.Lines().Len() > 1 {
		return r.renderSetextHeading(w, source, n, entering)
	}
	// Otherwise it's up to the configuration
	if r.HeadingStyle.IsSetext() {
		return r.renderSetextHeading(w, source, n, entering)
	}
	return r.renderATXHeading(w, source, n, entering)
}

func (r *Renderer) renderATXHeading(w util.BufWriter, source []byte, node *ast.Heading, entering bool) (ast.WalkStatus, error) {
	if entering {
		atxHeadingChars := strings.Repeat("#", node.Level)
		fmt.Fprint(w, atxHeadingChars)
		// Only print space after heading if non-empty
		if node.HasChildren() {
			fmt.Fprint(w, " ")
		}
	} else {
		if r.HeadingStyle == HeadingStyleATXSurround {
			atxHeadingChars := strings.Repeat("#", node.Level)
			fmt.Fprintf(w, " %v", atxHeadingChars)
		}
		r.renderBlockSeparator(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderSetextHeading(w util.BufWriter, source []byte, node *ast.Heading, entering bool) (ast.WalkStatus, error) {
	if entering {
		return ast.WalkContinue, nil
	}
	underlineChar := [...]string{"", "=", "-"}[node.Level]
	underlineWidth := 3
	if r.HeadingStyle == HeadingStyleFullWidthSetext {
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			lineWidth := line.Len()

			if lineWidth > underlineWidth {
				underlineWidth = lineWidth
			}
		}
	}
	fmt.Fprintf(w, "\n%v", strings.Repeat(underlineChar, underlineWidth))
	r.renderBlockSeparator(w, source, node)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderParagraph(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// If there is more content after this paragraph, close block with blank line
	if !entering {
		r.renderBlockSeparator(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Text)
	if entering {
		r.writer.Write(w, n.Text(source))
		if n.SoftLineBreak() {
			r.writer.Write(w, []byte("\n"))
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderThematicBreak(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		breakChar := [...]string{"-", "*", "_"}[r.ThematicBreakStyle]
		var breakLen int
		if r.ThematicBreakLength < ThematicBreakLengthMinimum {
			breakLen = int(ThematicBreakLengthMinimum)
		} else {
			breakLen = int(r.ThematicBreakLength)
		}
		thematicBreak := []byte(strings.Repeat(breakChar, breakLen))
		r.writer.Write(w, thematicBreak)
	} else {
		r.renderBlockSeparator(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.CodeBlock)
	if entering {
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			r.writer.Write(w, r.IndentStyle.bytes())
			r.writer.Write(w, line.Value(source))
		}
	} else {
		r.renderBlockSeparator(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	r.writer.Write(w, []byte("```"))
	if entering {
		if lang := n.Language(source); lang != nil {
			r.writer.Write(w, lang)
		}
		r.writer.Write(w, []byte("\n"))
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			r.writer.Write(w, line.Value(source))
		}
	} else {
		r.renderBlockSeparator(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.HTMLBlock)
	if entering {
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			r.writer.Write(w, line.Value(source))
		}
	} else {
		if n.HasClosure() {
			closure := n.ClosureLine
			r.writer.Write(w, closure.Value(source))
		}
		r.renderBlockSeparator(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderBlockSeparator(w util.BufWriter, source []byte, node ast.Node) {
	// If there is more content after this block, add empty line between blocks
	if node.NextSibling() != nil {
		r.writer.Write(w, []byte("\n\n"))
	}
}

// Writer interface is used to proxy write calls to the given util.BufWriter
type Writer interface {
	// Write writes the bytes from source to the given writer.
	Write(writer util.BufWriter, source []byte)
	// LastWriteBytes returns the bytes and length of the last write operation.
	LastWriteBytes() ([]byte, int)
}

type defaultWriter struct {
	// lastWriteBytes holds the contents of the last write operation.
	lastWriteBytes []byte
	// lastWriteLen is the length of the last write operation.
	lastWriteLen int
}

func (d *defaultWriter) Write(writer util.BufWriter, source []byte) {
	d.lastWriteBytes = source
	d.lastWriteLen, _ = writer.Write(source)
}

func (d *defaultWriter) LastWriteBytes() ([]byte, int) {
	return d.lastWriteBytes, d.lastWriteLen
}
