package ingest

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type SourceBindingAssessment struct {
	Runnable        bool
	Reason          string
	Detail          string
	ResolvedPath    string
	PersistPath     string
	ResolvedBaseURL string
	Target          *SourceBindingTarget
	Diagnostic      *SourceBindingDiagnostic
}

func cloneSourceBindingTarget(item *SourceBindingTarget) *SourceBindingTarget {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func AssessSourceBinding(source string, binding SourceBinding) SourceBindingAssessment {
	return assessSourceBinding(source, binding, "")
}

func assessSourceBinding(source string, binding SourceBinding, commentDataRoot string) SourceBindingAssessment {
	source = canonicalSource(firstNonEmpty(source, binding.Source))

	switch source {
	case RunSourceWXCommentExport:
		path := normalizeBindingPath(binding.Path)
		if path == "" {
			return SourceBindingAssessment{
				Runnable: false,
				Reason:   "缺少评论导出文件路径，请重新导入评论导出文件",
			}
		}

		resolution, err := resolveWXCommentExportPathWithRoot(path, commentDataRoot)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return SourceBindingAssessment{
					Runnable: false,
					Reason:   "评论导出文件不存在，请重新导入评论导出文件或更新路径",
				}
			}
			if errors.Is(err, errWXCommentExportNoUsableFile) {
				return SourceBindingAssessment{
					Runnable: false,
					Reason:   "评论导出目录下没有可用 JSON，请先在 wx_channel 重新导出评论",
				}
			}
			return SourceBindingAssessment{
				Runnable: false,
				Reason:   "评论导出文件当前不可读，请检查路径和权限后再试",
			}
		}
		payload, err := LoadWXCommentExportFile(resolution.ResolvedPath)
		if err != nil {
			return SourceBindingAssessment{
				Runnable: false,
				Reason:   "评论导出文件当前不可读，请检查路径和权限后再试",
			}
		}
		if isWXCommentExportProofPlaceholder(firstNonEmpty(resolution.PersistPath, resolution.ResolvedPath, path), payload) {
			return SourceBindingAssessment{
				Runnable:     false,
				Reason:       "当前保存的是 team runtime proof 占位评论导出，不是正式评论来源。请在 wx_channel 中对真实视频重新导出评论 JSON，然后重新绑定这条来源。",
				ResolvedPath: resolution.ResolvedPath,
				PersistPath:  resolution.PersistPath,
			}
		}
		return SourceBindingAssessment{
			Runnable:     true,
			ResolvedPath: resolution.ResolvedPath,
			PersistPath:  resolution.PersistPath,
			Target:       buildSourceBindingTargetFromCommentExport(payload),
		}

	case RunSourceWXFeedList:
		resolvedBaseURL := normalizeWXFeedListBaseURL(binding.BaseURL)
		if strings.TrimSpace(binding.Username) == "" {
			return SourceBindingAssessment{
				Runnable:        false,
				Reason:          "缺少视频号 username，请重新导入视频列表来源",
				ResolvedBaseURL: resolvedBaseURL,
				Diagnostic: &SourceBindingDiagnostic{
					Code:            "wx_feed_list_missing_username",
					ResolvedBaseURL: resolvedBaseURL,
				},
			}
		}
		return SourceBindingAssessment{
			Runnable:        true,
			ResolvedBaseURL: resolvedBaseURL,
		}

	case RunSourceWXVideoSearch:
		resolvedBaseURL := normalizeWXFeedListBaseURL(binding.BaseURL)
		if strings.TrimSpace(binding.Keyword) == "" {
			return SourceBindingAssessment{
				Runnable:        false,
				Reason:          "缺少视频搜索关键词，请重新保存或导入关键词视频来源",
				ResolvedBaseURL: resolvedBaseURL,
				Diagnostic: &SourceBindingDiagnostic{
					Code:            "wx_video_search_missing_keyword",
					ResolvedBaseURL: resolvedBaseURL,
				},
			}
		}
		return SourceBindingAssessment{
			Runnable:        true,
			ResolvedBaseURL: resolvedBaseURL,
		}

	case RunSourceWXManualShare:
		if strings.TrimSpace(binding.ShareURL) == "" {
			return SourceBindingAssessment{
				Runnable: false,
				Reason:   "缺少视频号分享链接，无法继续采集这条手动视频",
			}
		}
		return SourceBindingAssessment{
			Runnable:        true,
			ResolvedBaseURL: strings.TrimSpace(binding.BaseURL),
		}

	case "":
		return SourceBindingAssessment{
			Runnable: false,
			Reason:   "保存的来源类型为空，请重新导入该来源",
		}

	default:
		return SourceBindingAssessment{
			Runnable: false,
			Reason:   fmt.Sprintf("保存的来源类型 %s 暂不支持自动重试", source),
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneSourceBindingDiagnostic(item *SourceBindingDiagnostic) *SourceBindingDiagnostic {
	if item == nil {
		return nil
	}
	copy := *item
	if item.ConnectedClientCount != nil {
		value := *item.ConnectedClientCount
		copy.ConnectedClientCount = &value
	}
	if item.ReadyClientCount != nil {
		value := *item.ReadyClientCount
		copy.ReadyClientCount = &value
	}
	if item.FeedReadyClientCount != nil {
		value := *item.FeedReadyClientCount
		copy.FeedReadyClientCount = &value
	}
	return &copy
}
