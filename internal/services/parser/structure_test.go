package parser

import "testing"

func TestDetectStructureFromNestedTOC(t *testing.T) {
	chapters := []RawChapter{
		{Title: "序章", VolumeTitle: "第一卷 初入江湖"},
		{Title: "第一章 山边小村", VolumeTitle: "第一卷 初入江湖"},
		{Title: "第二章 青牛镇", VolumeTitle: "第二卷 风起云涌"},
	}

	volumes, gotChapters := DetectStructure(chapters, true)

	assertVolumes(t, volumes, []VolumeInfo{
		{Title: "第一卷 初入江湖", Seq: 1, DetectSource: "toc"},
		{Title: "第二卷 风起云涌", Seq: 2, DetectSource: "toc"},
	})
	assertVolumeSeqs(t, gotChapters, []int{1, 1, 2})
	assertInputUntouched(t, chapters)
}

func TestDetectStructureFromKeywords(t *testing.T) {
	chapters := []RawChapter{
		{Title: "楔子"},
		{Title: "第一卷 初入江湖"},
		{Title: "第一章 山边小村"},
		{Title: "第二卷 风起云涌"},
		{Title: "第二章 再入江湖"},
	}

	volumes, gotChapters := DetectStructure(chapters, false)

	assertVolumes(t, volumes, []VolumeInfo{
		{Title: "第一卷 初入江湖", Seq: 1, DetectSource: "keyword"},
		{Title: "第二卷 风起云涌", Seq: 2, DetectSource: "keyword"},
	})
	assertVolumeSeqs(t, gotChapters, []int{0, 1, 1, 2, 2})
	assertInputUntouched(t, chapters)
}

func TestDetectStructureFromVolumeField(t *testing.T) {
	chapters := []RawChapter{
		{Title: "第一章", VolumeTitle: "上篇 少年行"},
		{Title: "第二章", VolumeTitle: "上篇 少年行"},
		{Title: "第三章", VolumeTitle: "下篇 天地变"},
		{Title: "尾声"},
	}

	volumes, gotChapters := DetectStructure(chapters, false)

	assertVolumes(t, volumes, []VolumeInfo{
		{Title: "上篇 少年行", Seq: 1, DetectSource: "keyword"},
		{Title: "下篇 天地变", Seq: 2, DetectSource: "keyword"},
	})
	assertVolumeSeqs(t, gotChapters, []int{1, 1, 2, 0})
	assertInputUntouched(t, chapters)
}

func TestDetectStructureWithoutVolumes(t *testing.T) {
	chapters := []RawChapter{
		{Title: "第一章 山边小村"},
		{Title: "第二章 青牛镇"},
	}

	volumes, gotChapters := DetectStructure(chapters, false)

	if len(volumes) != 0 {
		t.Fatalf("expected no volumes, got %d", len(volumes))
	}
	if volumes == nil {
		t.Fatal("expected empty volumes slice, got nil")
	}
	assertVolumeSeqs(t, gotChapters, []int{0, 0})
	assertInputUntouched(t, chapters)
}

func assertVolumes(t *testing.T, got, want []VolumeInfo) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("volume count = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("volume[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func assertVolumeSeqs(t *testing.T, chapters []RawChapter, want []int) {
	t.Helper()

	if len(chapters) != len(want) {
		t.Fatalf("chapter count = %d, want %d", len(chapters), len(want))
	}

	for i := range want {
		if chapters[i].VolumeSeq != want[i] {
			t.Fatalf("chapter[%d].VolumeSeq = %d, want %d", i, chapters[i].VolumeSeq, want[i])
		}
	}
}

func assertInputUntouched(t *testing.T, chapters []RawChapter) {
	t.Helper()

	for i, ch := range chapters {
		if ch.VolumeSeq != 0 {
			t.Fatalf("input chapter[%d] mutated: VolumeSeq = %d", i, ch.VolumeSeq)
		}
	}
}
