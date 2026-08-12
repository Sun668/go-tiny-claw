package context

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Body        string
	RelDir      string
	RelPath     string
	AbsDir      string
}

type SkillLoader struct {
	workDir string
}

func NewSkillLoader(workDir string) *SkillLoader {
	return &SkillLoader{workDir: workDir}
}

func (s *SkillLoader) baseDir() string {
	return filepath.Join(s.workDir, ".claw", "skills")
}

func (s *SkillLoader) List() []Skill {
	baseDir := s.baseDir()
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil
	}

	var skills []Skill
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		skill := parseSkillMD(string(content))
		absDir := filepath.Dir(path)
		if skill.Name == "" || skill.Name == "未命名技能" {
			skill.Name = filepath.Base(absDir)
		}
		skill.AbsDir = absDir
		skill.RelDir, _ = filepath.Rel(s.workDir, absDir)
		skill.RelPath, _ = filepath.Rel(s.workDir, path)
		skill.RelDir = filepath.ToSlash(skill.RelDir)
		skill.RelPath = filepath.ToSlash(skill.RelPath)
		skill.Body = ""
		skills = append(skills, skill)
		return nil
	})

	return skills
}

func (s *SkillLoader) Load(name string) (Skill, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Skill{}, nil, fmt.Errorf("技能名称不能为空")
	}

	for _, skill := range s.List() {
		if !strings.EqualFold(skill.Name, name) && !strings.EqualFold(filepath.Base(skill.RelDir), name) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(s.workDir, filepath.FromSlash(skill.RelPath)))
		if err != nil {
			return Skill{}, nil, fmt.Errorf("读取技能失败: %w", err)
		}

		loaded := parseSkillMD(string(content))
		loaded.Name = skill.Name
		loaded.AbsDir = skill.AbsDir
		loaded.RelDir = skill.RelDir
		loaded.RelPath = skill.RelPath
		if loaded.Description == "" || loaded.Description == "未提供描述" {
			loaded.Description = skill.Description
		}

		return loaded, listCompanions(s.workDir, skill.AbsDir), nil
	}

	return Skill{}, nil, fmt.Errorf("未找到名为 '%s' 的技能", name)
}

func (s *SkillLoader) CatalogPrompt() string {
	skills := s.List()
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n### 可用专业技能 (Agent Skills)\n")
	b.WriteString("技能采用按需加载：下面只列出名称和触发条件。当用户任务匹配某个技能时，你必须先调用 `read_skill` 读取完整执行指南，再严格遵循；不要凭记忆猜测技能内容。同目录附件需要时再用 `read_file` 读取。\n\n")

	for _, skill := range skills {
		b.WriteString(fmt.Sprintf("- **%s**（`%s`）：%s\n", skill.Name, skill.RelPath, skill.Description))
	}

	return b.String()
}

func listCompanions(workDir, absDir string) []string {
	var companions []string
	_ = filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "SKILL.md" {
			return err
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}
		companions = append(companions, filepath.ToSlash(rel))
		return nil
	})
	return companions
}

func parseSkillMD(content string) Skill {
	skill := Skill{
		Name:        "未命名技能",
		Description: "未提供描述",
		Body:        strings.TrimSpace(content),
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return skill
	}

	parts := strings.SplitN(normalized, "---", 3)
	if len(parts) != 3 {
		return skill
	}

	skill.Body = strings.TrimSpace(parts[2])
	name, description := parseFrontmatter(parts[1])
	if name != "" {
		skill.Name = name
	}
	if description != "" {
		skill.Description = description
	}
	return skill
}

func parseFrontmatter(frontmatter string) (name, description string) {
	lines := strings.Split(frontmatter, "\n")
	currentKey := ""
	var descParts []string

	flushDescription := func() {
		if len(descParts) > 0 {
			description = strings.TrimSpace(strings.Join(descParts, " "))
		}
	}

	for _, line := range lines {
		if currentKey == "description" && isIndented(line) {
			if text := strings.TrimSpace(line); text != "" {
				descParts = append(descParts, text)
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		key, value, ok := splitYAMLField(trimmed)
		if !ok {
			if currentKey == "description" && trimmed != "" {
				descParts = append(descParts, trimmed)
			}
			continue
		}

		if currentKey == "description" && key != "description" {
			flushDescription()
			currentKey = ""
		}

		switch key {
		case "name":
			currentKey = "name"
			if value != "" {
				name = unquoteYAML(value)
			}
		case "description":
			currentKey = "description"
			if isYAMLFoldIndicator(value) {
				descParts = nil
				continue
			}
			if value != "" {
				descParts = []string{unquoteYAML(value)}
			}
		default:
			currentKey = ""
		}
	}

	if currentKey == "description" {
		flushDescription()
	}

	return name, description
}

func splitYAMLField(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func isYAMLFoldIndicator(value string) bool {
	switch value {
	case ">", ">-", "|", "|-", "|+":
		return true
	default:
		return false
	}
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func unquoteYAML(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}
