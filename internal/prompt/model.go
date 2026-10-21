package prompt

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	KeyTopicAnalysis      = "topic-analysis"
	KeyTopicScript        = "topic-script"
	KeyContentAnalysis    = "content-analysis"
	KeyTopicTrends        = "topic-trends"
	KeyAccountPerformance = "account-performance"
	KeyCommentInsights    = "comment-insights"
	KeyDailyReport        = "daily-report"

	IntegrationActive  = "active"
	IntegrationPending = "pending"

	DefaultVersion = "builtin-2026-08-26"
)

var templateVariablePattern = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_]*)\s*\}\}`)

type Profile struct {
	Key                string    `json:"key"`
	Label              string    `json:"label"`
	Description        string    `json:"description"`
	SystemPrompt       string    `json:"systemPrompt"`
	UserPromptTemplate string    `json:"userPromptTemplate"`
	Version            string    `json:"version"`
	Enabled            bool      `json:"enabled"`
	IntegrationState   string    `json:"integrationState"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type Settings struct {
	Profiles  map[string]Profile `json:"profiles"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type ProfilePatch struct {
	SystemPrompt       string `json:"systemPrompt"`
	UserPromptTemplate string `json:"userPromptTemplate"`
	Enabled            *bool  `json:"enabled,omitempty"`
}

type Repository interface {
	Load(context.Context, string) (Settings, error)
	Save(context.Context, string, Settings) error
}

type ProfileDescriptor struct {
	Key                       string
	Label                     string
	Description               string
	DefaultSystemPrompt       string
	DefaultUserPromptTemplate string
	IntegrationState          string
	DefaultEnabled            bool
	Variables                 []string
}

var descriptors = map[string]ProfileDescriptor{
	KeyContentAnalysis: {
		Key:                       KeyContentAnalysis,
		Label:                     "视频分析",
		Description:               "用于视频表现、内容结构和发布表现的分析建议。",
		DefaultSystemPrompt:       "你是内容数据平台的内容表现分析师。只根据输入的数据和证据做判断，区分事实、推断和待验证假设，不得补造指标。输出应适合运营团队执行。",
		DefaultUserPromptTemplate: "请分析以下内容数据 JSON，并给出可复核的内容结论：\n{{data_json}}",
		IntegrationState:          IntegrationActive,
		DefaultEnabled:            true,
		Variables:                 []string{"data_json", "video_json"},
	},
	KeyTopicTrends: {
		Key:                       KeyTopicTrends,
		Label:                     "关键词样本分析",
		Description:               "用于话题规范化、趋势判断和来源证据解释。",
		DefaultSystemPrompt:       "你是话题趋势分析师。只使用输入中的真实话题、来源和时间窗口，规范表达但不改变原始事实。所有趋势结论都必须说明依据，无法确认时标记待验证。",
		DefaultUserPromptTemplate: "请分析以下话题趋势数据 JSON：\n{{data_json}}",
		IntegrationState:          IntegrationActive,
		DefaultEnabled:            true,
		Variables:                 []string{"data_json"},
	},
	KeyAccountPerformance: {
		Key:                       KeyAccountPerformance,
		Label:                     "账号表现",
		Description:               "用于账号画像、内容表现和重点账号判断。",
		DefaultSystemPrompt:       "你是账号运营分析师。仅依据输入的账号资料、视频数据和时间窗口输出判断。地区、行业、标签等信息缺失时必须明确标注，不得猜测。",
		DefaultUserPromptTemplate: "请分析以下账号表现数据 JSON：\n{{account_json}}",
		IntegrationState:          IntegrationActive,
		DefaultEnabled:            true,
		Variables:                 []string{"data_json", "account_json"},
	},
	KeyCommentInsights: {
		Key:                       KeyCommentInsights,
		Label:                     "评论洞察",
		Description:               "用于评论主题、问题密度、情绪和内容机会分析。",
		DefaultSystemPrompt:       "你是评论洞察分析师。只根据真实评论内容和统计数据归纳问题、需求和情绪，不得把少量评论扩大为整体结论。引用证据并明确样本边界。",
		DefaultUserPromptTemplate: "请分析以下评论洞察数据 JSON：\n{{comments_json}}",
		IntegrationState:          IntegrationActive,
		DefaultEnabled:            true,
		Variables:                 []string{"data_json", "comments_json"},
	},
}

func Descriptor(key string) (ProfileDescriptor, bool) {
	descriptor, ok := descriptors[strings.TrimSpace(key)]
	if !ok {
		return ProfileDescriptor{}, false
	}
	descriptor.Variables = append([]string(nil), descriptor.Variables...)
	return descriptor, true
}

func Descriptors() []ProfileDescriptor {
	keys := make([]string, 0, len(descriptors))
	for key := range descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]ProfileDescriptor, 0, len(keys))
	for _, key := range keys {
		descriptor, _ := Descriptor(key)
		items = append(items, descriptor)
	}
	return items
}

func DefaultProfile(key string) (Profile, bool) {
	descriptor, ok := Descriptor(key)
	if !ok {
		return Profile{}, false
	}
	return Profile{
		Key:                descriptor.Key,
		Label:              descriptor.Label,
		Description:        descriptor.Description,
		SystemPrompt:       descriptor.DefaultSystemPrompt,
		UserPromptTemplate: descriptor.DefaultUserPromptTemplate,
		Version:            DefaultVersion,
		Enabled:            descriptor.DefaultEnabled,
		IntegrationState:   descriptor.IntegrationState,
	}, true
}

func DefaultSettings() Settings {
	items := make(map[string]Profile, len(descriptors))
	for _, descriptor := range Descriptors() {
		profile, _ := DefaultProfile(descriptor.Key)
		items[descriptor.Key] = profile
	}
	return Settings{Profiles: items}
}

func NormalizeSettings(settings Settings) Settings {
	defaults := DefaultSettings()
	if settings.Profiles == nil {
		settings.Profiles = make(map[string]Profile, len(defaults.Profiles))
	}
	for key, fallback := range defaults.Profiles {
		profile, ok := settings.Profiles[key]
		if !ok {
			settings.Profiles[key] = fallback
			continue
		}
		profile.Key = key
		profile.Label = fallback.Label
		profile.Description = fallback.Description
		profile.IntegrationState = fallback.IntegrationState
		if strings.TrimSpace(profile.Version) == "" {
			profile.Version = fallback.Version
		}
		if strings.TrimSpace(profile.SystemPrompt) == "" {
			profile.SystemPrompt = fallback.SystemPrompt
		}
		if strings.TrimSpace(profile.UserPromptTemplate) == "" {
			profile.UserPromptTemplate = fallback.UserPromptTemplate
		}
		settings.Profiles[key] = profile
	}
	return settings
}

func ApplyPatch(settings Settings, key string, patch ProfilePatch, now time.Time) (Settings, error) {
	settings = NormalizeSettings(settings)
	if _, ok := Descriptor(key); !ok {
		return Settings{}, fmt.Errorf("unknown prompt profile %q", key)
	}
	profile, ok := settings.Profiles[strings.TrimSpace(key)]
	if !ok {
		return Settings{}, fmt.Errorf("unknown prompt profile %q", key)
	}
	profile.SystemPrompt = patch.SystemPrompt
	profile.UserPromptTemplate = patch.UserPromptTemplate
	if patch.Enabled != nil {
		profile.Enabled = *patch.Enabled
	}
	if err := ValidateProfile(profile); err != nil {
		return Settings{}, err
	}
	profile.Version = nextVersion(profile.Version)
	profile.UpdatedAt = now.UTC()
	settings.Profiles[profile.Key] = profile
	settings.UpdatedAt = now.UTC()
	return settings, nil
}

func ResetProfile(settings Settings, key string, now time.Time) (Settings, error) {
	settings = NormalizeSettings(settings)
	profile, ok := DefaultProfile(key)
	if !ok {
		return Settings{}, fmt.Errorf("unknown prompt profile %q", key)
	}
	profile.Version = nextVersion(settings.Profiles[key].Version)
	profile.UpdatedAt = now.UTC()
	settings.Profiles[key] = profile
	settings.UpdatedAt = now.UTC()
	return settings, nil
}

func ValidateSettings(settings Settings) error {
	settings = NormalizeSettings(settings)
	for key, profile := range settings.Profiles {
		if _, active := Descriptor(key); !active {
			continue // Preserve historical profiles without exposing or executing them.
		}
		if key != profile.Key {
			return fmt.Errorf("prompt profile key mismatch: %q", key)
		}
		if err := ValidateProfile(profile); err != nil {
			return err
		}
	}
	return nil
}

func ValidateProfile(profile Profile) error {
	descriptor, ok := Descriptor(profile.Key)
	if !ok {
		return fmt.Errorf("unknown prompt profile %q", profile.Key)
	}
	if strings.TrimSpace(profile.SystemPrompt) == "" {
		return errors.New("system prompt is required")
	}
	if strings.TrimSpace(profile.UserPromptTemplate) == "" {
		return errors.New("user prompt template is required")
	}
	if len(profile.SystemPrompt) > 32000 || len(profile.UserPromptTemplate) > 64000 {
		return errors.New("prompt text is too large")
	}
	allowed := make(map[string]struct{}, len(descriptor.Variables))
	for _, variable := range descriptor.Variables {
		allowed[variable] = struct{}{}
	}
	for _, match := range templateVariablePattern.FindAllStringSubmatch(profile.UserPromptTemplate, -1) {
		if _, ok := allowed[match[1]]; !ok {
			return fmt.Errorf("unsupported template variable %q for %s", match[1], profile.Key)
		}
	}
	return nil
}

func Render(template string, values map[string]string) (string, error) {
	var err error
	rendered := templateVariablePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := templateVariablePattern.FindStringSubmatch(match)
		value, ok := values[parts[1]]
		if !ok {
			err = fmt.Errorf("missing template variable %q", parts[1])
			return match
		}
		return value
	})
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func nextVersion(current string) string {
	current = strings.TrimSpace(current)
	if current == "" || current == DefaultVersion {
		return "v2"
	}
	if strings.HasPrefix(current, "v") {
		var number int
		if _, err := fmt.Sscanf(current, "v%d", &number); err == nil && number > 0 {
			return fmt.Sprintf("v%d", number+1)
		}
	}
	return "v2"
}
