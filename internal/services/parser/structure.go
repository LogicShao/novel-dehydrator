package parser

import "regexp"

var volumeKeywords = regexp.MustCompile(`[篇卷部册]`)

type RawChapter struct {
	Title       string
	Content     string
	VolumeSeq   int
	VolumeTitle string
}

type VolumeInfo struct {
	Title        string
	Seq          int
	DetectSource string
}

// DetectStructure mirrors the Python structure detector.
// Detection priority: nested TOC -> title keywords -> preset volume field -> no volume structure.
func DetectStructure(chapters []RawChapter, hasNestedTOC bool) ([]VolumeInfo, []RawChapter) {
	if hasNestedTOC {
		return fromVolumeField(chapters, "toc")
	}

	volumePositions := make([]volumePosition, 0)
	for i, ch := range chapters {
		if volumeKeywords.MatchString(ch.Title) && len([]rune(ch.Title)) <= 25 {
			volumePositions = append(volumePositions, volumePosition{index: i, title: ch.Title})
		}
	}

	if len(volumePositions) > 0 {
		return fromVolumePositions(chapters, volumePositions, "keyword")
	}

	hasVolumeField := false
	for _, ch := range chapters {
		if ch.VolumeTitle != "" {
			hasVolumeField = true
			break
		}
	}

	if hasVolumeField {
		return fromVolumeField(chapters, "keyword")
	}

	return []VolumeInfo{}, cloneChapters(chapters)
}

type volumePosition struct {
	index int
	title string
}

func fromVolumeField(chapters []RawChapter, source string) ([]VolumeInfo, []RawChapter) {
	result := cloneChapters(chapters)
	volumes := make([]VolumeInfo, 0)
	volSeqMap := make(map[string]int)
	seq := 0

	for i, ch := range result {
		volTitle := ch.VolumeTitle
		if volTitle != "" {
			if _, exists := volSeqMap[volTitle]; !exists {
				seq++
				volSeqMap[volTitle] = seq
				volumes = append(volumes, VolumeInfo{
					Title:        volTitle,
					Seq:          seq,
					DetectSource: source,
				})
			}
			volumeSeq := volSeqMap[volTitle]
			result[i].VolumeSeq = volumeSeq
			continue
		}

		result[i].VolumeSeq = 0
	}

	return volumes, result
}

func fromVolumePositions(chapters []RawChapter, positions []volumePosition, source string) ([]VolumeInfo, []RawChapter) {
	result := cloneChapters(chapters)
	volumes := make([]VolumeInfo, 0, len(positions))
	seq := 0
	breaks := make([]int, 0, len(positions)+1)

	for _, pos := range positions {
		breaks = append(breaks, pos.index)
	}
	breaks = append(breaks, len(result))

	for idx, pos := range positions {
		seq++
		volumes = append(volumes, VolumeInfo{
			Title:        pos.title,
			Seq:          seq,
			DetectSource: source,
		})

		end := breaks[idx+1]
		for i := pos.index; i < end; i++ {
			result[i].VolumeSeq = seq
			result[i].VolumeTitle = pos.title
		}
	}

	if len(breaks) > 0 {
		for i := 0; i < breaks[0]; i++ {
			result[i].VolumeSeq = 0
			result[i].VolumeTitle = ""
		}
	}

	return volumes, result
}

func cloneChapters(chapters []RawChapter) []RawChapter {
	if chapters == nil {
		return nil
	}

	result := make([]RawChapter, len(chapters))
	copy(result, chapters)
	return result
}
