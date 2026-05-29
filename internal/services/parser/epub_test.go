package parser

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseEPUB(t *testing.T) {
	t.Run("ncx nested toc with dedupe", func(t *testing.T) {
		book := parseFromTestArchive(t, map[string]string{
			"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
			"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookId">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>测试 EPUB</dc:title>
    <dc:creator>张三</dc:creator>
  </metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="chap1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="chap2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="chap1"/>
    <itemref idref="chap2"/>
  </spine>
</package>`,
			"OEBPS/toc.ncx": `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="vol1" playOrder="1">
      <navLabel><text>第一卷 初入江湖</text></navLabel>
      <navPoint id="ch1" playOrder="2">
        <navLabel><text>第一章 山边小村</text></navLabel>
        <content src="chapter1.xhtml#start"/>
      </navPoint>
      <navPoint id="dup" playOrder="3">
        <navLabel><text>第一章 山边小村</text></navLabel>
        <content src="chapter1.xhtml#dup"/>
      </navPoint>
    </navPoint>
    <navPoint id="ch2" playOrder="4">
      <navLabel><text>第二章 青牛镇</text></navLabel>
      <content src="chapter2.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`,
			"OEBPS/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
  <div>第一章标题</div>
  <p>山边小村的故事开始。</p>
  <script>ignored()</script>
  <p>少年踏上旅程。</p>
</body></html>`,
			"OEBPS/chapter2.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
  <p>青牛镇风云再起。</p>
</body></html>`,
		})

		if book.Title != "测试 EPUB" {
			t.Fatalf("Title = %q, want %q", book.Title, "测试 EPUB")
		}
		if book.Author != "张三" {
			t.Fatalf("Author = %q, want %q", book.Author, "张三")
		}
		if !book.HasNestedTOC {
			t.Fatal("HasNestedTOC = false, want true")
		}
		if len(book.Chapters) != 2 {
			t.Fatalf("len(Chapters) = %d, want 2", len(book.Chapters))
		}

		first := book.Chapters[0]
		if first.Title != "第一章 山边小村" {
			t.Fatalf("first.Title = %q", first.Title)
		}
		if first.Volume != "第一卷 初入江湖" {
			t.Fatalf("first.Volume = %q", first.Volume)
		}
		if strings.Contains(first.Text, "ignored") {
			t.Fatalf("first.Text should exclude script content: %q", first.Text)
		}
		if !strings.Contains(first.Text, "山边小村的故事开始。") || !strings.Contains(first.Text, "少年踏上旅程。") {
			t.Fatalf("first.Text missing content: %q", first.Text)
		}
	})

	t.Run("nav toc fallback title and text extraction", func(t *testing.T) {
		book := parseFromTestArchive(t, map[string]string{
			"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OPS/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
			"OPS/package.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title></dc:title>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="chapter" href="text/chapter-01.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter"/>
  </spine>
</package>`,
			"OPS/nav.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <body>
    <nav epub:type="toc">
      <ol>
        <li><a href="text/chapter-01.xhtml">序章</a></li>
      </ol>
    </nav>
  </body>
</html>`,
			"OPS/text/chapter-01.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
  <section><h1>隐式标题</h1><p>只有一章内容。</p></section>
</body></html>`,
		})

		if book.Title != "未知书名" {
			t.Fatalf("Title = %q, want 未知书名", book.Title)
		}
		if book.HasNestedTOC {
			t.Fatal("HasNestedTOC = true, want false")
		}
		if len(book.Chapters) != 1 {
			t.Fatalf("len(Chapters) = %d, want 1", len(book.Chapters))
		}
		if book.Chapters[0].Title != "序章" {
			t.Fatalf("chapter title = %q, want 序章", book.Chapters[0].Title)
		}
		if strings.TrimSpace(book.Chapters[0].Text) != "隐式标题\n\n只有一章内容。" {
			t.Fatalf("chapter text = %q", book.Chapters[0].Text)
		}
	})

	t.Run("br separates epub paragraphs", func(t *testing.T) {
		book := parseFromTestArchive(t, map[string]string{
			"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OPS/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
			"OPS/package.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>换行测试</dc:title>
  </metadata>
  <manifest>
    <item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter"/>
  </spine>
</package>`,
			"OPS/chapter.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
  <p>第一段<br/>第二段<br />第三段</p>
</body></html>`,
		})

		if len(book.Chapters) != 1 {
			t.Fatalf("len(Chapters) = %d, want 1", len(book.Chapters))
		}
		if strings.TrimSpace(book.Chapters[0].Text) != "第一段\n\n第二段\n\n第三段" {
			t.Fatalf("chapter text = %q", book.Chapters[0].Text)
		}
	})

	t.Run("metadata prefix fallback and nested volume propagation", func(t *testing.T) {
		book := parseFromTestArchive(t, map[string]string{
			"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
			"OPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>前缀标题</dc:title>
    <dc:creator>前缀作者</dc:creator>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="chapter" href="text/chapter.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter"/>
  </spine>
</package>`,
			"OPS/nav.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <body>
    <nav epub:type="toc">
      <ol>
        <li>
          <span>卷一</span>
          <ol>
            <li><a href="text/chapter.xhtml">第一章</a></li>
          </ol>
        </li>
      </ol>
    </nav>
  </body>
</html>`,
			"OPS/text/chapter.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>正文</body></html>`,
		})

		if book.Title != "前缀标题" {
			t.Fatalf("Title = %q, want 前缀标题", book.Title)
		}
		if book.Author != "前缀作者" {
			t.Fatalf("Author = %q, want 前缀作者", book.Author)
		}
		if len(book.Chapters) != 1 {
			t.Fatalf("len(Chapters) = %d, want 1", len(book.Chapters))
		}
		if book.Chapters[0].Volume != "卷一" {
			t.Fatalf("Volume = %q, want 卷一", book.Chapters[0].Volume)
		}
	})

	t.Run("ncx href already rooted at opf dir", func(t *testing.T) {
		book := parseFromTestArchive(t, map[string]string{
			"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
			"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>NCX 路径测试</dc:title>
  </metadata>
  <manifest>
    <item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>
    <item id="toc" href="OEBPS/toc.ncx" media-type="application/x-dtbncx+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter"/>
  </spine>
</package>`,
			"OEBPS/toc.ncx": `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="ch1" playOrder="1">
      <navLabel><text>第一章</text></navLabel>
      <content src="chapter.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`,
			"OEBPS/chapter.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><p>正文</p></body></html>`,
		})

		if len(book.Chapters) != 1 {
			t.Fatalf("len(Chapters) = %d, want 1", len(book.Chapters))
		}
		if book.Chapters[0].Title != "第一章" {
			t.Fatalf("Title = %q, want 第一章", book.Chapters[0].Title)
		}
	})
}

func parseFromTestArchive(t *testing.T, files map[string]string) *EPUBBook {
	t.Helper()

	data := buildTestEPUB(t, files)
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader failed: %v", err)
	}

	book, err := parseEPUBFromZip(reader)
	if err != nil {
		t.Fatalf("parseEPUBFromZip failed: %v", err)
	}
	return book
}

func buildTestEPUB(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) failed: %v", name, err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) failed: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close failed: %v", err)
	}
	return buffer.Bytes()
}
