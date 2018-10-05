package existingvolumebroker

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"code.cloudfoundry.org/lager"
)

type ConfigDetails struct {
	Allowed []string

	Forced  map[string]string
	Options map[string]string
}

type Config struct {
	mount       ConfigDetails
	sloppyMount bool
}

func inArray(list []string, key string) bool {
	for _, k := range list {
		if k == key {
			return true
		}
	}

	return false
}

func NewExistingVolumeBrokerConfig(mountDetails *ConfigDetails) *Config {
	myConf := new(Config)

	myConf.mount = *mountDetails
	myConf.sloppyMount = false

	return myConf
}

func (rhs *Config) Copy() *Config {
	myConf := new(Config)

	myConf.mount = *rhs.mount.Copy()
	myConf.sloppyMount = rhs.sloppyMount
	return myConf
}

func NewExistingVolumeBrokerConfigDetails() *ConfigDetails {
	myConf := new(ConfigDetails)

	myConf.Allowed = make([]string, 0)
	myConf.Options = make(map[string]string, 0)
	myConf.Forced = make(map[string]string, 0)

	return myConf
}

func (rhs *ConfigDetails) Copy() *ConfigDetails {
	myConf := new(ConfigDetails)

	myConf.Allowed = rhs.Allowed

	myConf.Forced = make(map[string]string, 0)
	myConf.Options = make(map[string]string, 0)
	for k, v := range rhs.Forced {
		myConf.Forced[k] = v
	}
	for k, v := range rhs.Options {
		myConf.Options[k] = v
	}
	return myConf
}

func (m *Config) SetEntries(logger lager.Logger, share string, opts map[string]interface{}, ignoreList []string) error {
	m.mount.parseMap(logger, opts, ignoreList)

	allowed := append(ignoreList, m.mount.Allowed...)
	errorList := m.mount.parseUrl(share, ignoreList)
	m.sloppyMount = m.mount.IsSloppyMount()

	for k, _ := range opts {
		if !inArray(allowed, k) {
			errorList = append(errorList, k)
		}
	}

	if len(errorList) > 0 && m.sloppyMount != true {
		err := errors.New("Not allowed options: " + strings.Join(errorList, ", "))
		return err
	}

	return nil
}

func (m Config) Share(share string) string {
	srcPart := strings.SplitN(share, "?", 2)
	return srcPart[0]
}

func (m Config) MountConfig() map[string]interface{} {
	return m.mount.makeConfig()
}

func (m *ConfigDetails) readConfDefault(flagString string) {
	if len(flagString) < 1 {
		return
	}

	m.Options = m.parseConfig(strings.Split(flagString, ","))
	m.Forced = make(map[string]string)

	for k, v := range m.Options {
		if !inArray(m.Allowed, k) {
			m.Forced[k] = v
			delete(m.Options, k)
		}
	}
}

func (m *ConfigDetails) ReadConf(allowedFlag string, defaultFlag string) error {
	if len(allowedFlag) > 0 {
		m.Allowed = strings.Split(allowedFlag, ",")
	}

	m.readConfDefault(defaultFlag)

	return nil
}

func (m ConfigDetails) parseConfig(listEntry []string) map[string]string {

	result := map[string]string{}

	for _, opt := range listEntry {

		key := strings.SplitN(opt, ":", 2)

		if len(key[0]) < 1 {
			continue
		}

		if len(key[1]) < 1 {
			result[key[0]] = ""
		} else {
			result[key[0]] = key[1]
		}
	}

	return result
}

func (m *ConfigDetails) IsSloppyMount() bool {

	spm := ""
	ok := false

	if _, ok = m.Options["sloppy_mount"]; ok {
		spm = m.Options["sloppy_mount"]
		delete(m.Options, "sloppy_mount")
	}

	if _, ok = m.Forced["sloppy_mount"]; ok {
		spm = m.Forced["sloppy_mount"]
		delete(m.Forced, "sloppy_mount")
	}

	if len(spm) > 0 {
		if val, err := strconv.ParseBool(spm); err == nil {
			return val
		}
	}

	return false
}

func uniformKeyData(key string, data interface{}) string {
	switch key {
	case "auto-traverse-mounts":
		return uniformData(data, true)

	case "dircache":
		return uniformData(data, true)

	}

	return uniformData(data, false)
}

func uniformData(data interface{}, boolAsInt bool) string {

	switch data.(type) {
	case int, int8, int16, int32, int64, float32, float64:
		return fmt.Sprintf("%#v", data)

	case string:
		return data.(string)

	case bool:
		if boolAsInt {
			if data.(bool) {
				return "1"
			} else {
				return "0"
			}
		} else {
			return strconv.FormatBool(data.(bool))
		}
	}

	return ""
}

func (m *ConfigDetails) parseUrl(url string, ignoreList []string) []string {

	var errorList []string

	part := strings.SplitN(url, "?", 2)

	if len(part) < 2 {
		part = append(part, "")
	}

	for _, p := range strings.Split(part[1], "&") {
		if key := m.parseUrlParams(p, ignoreList); len(key) > 0 {
			errorList = append(errorList, key)
		}

	}

	return errorList
}

func (m *ConfigDetails) parseUrlParams(urlParams string, ignoreList []string) string {

	op := strings.SplitN(urlParams, "=", 2)

	if len(op) < 2 || len(op[1]) < 1 || op[1] == "" || inArray(ignoreList, op[0]) {
		return ""
	}

	if inArray(m.Allowed, op[0]) {
		m.Options[op[0]] = uniformKeyData(op[0], op[1])
		return ""
	}

	return op[0]
}

func (m *ConfigDetails) parseMap(logger lager.Logger, entryList map[string]interface{}, ignoreList []string) {

	for k, v := range entryList {

		value := uniformKeyData(k, v)

		if value == "" || len(k) < 1 || inArray(ignoreList, k) {
			continue
		}

		// Only set allowed options in the mount config
		if inArray(m.Allowed, k) {
			m.Options[k] = value
		}
	}
}

func (m *ConfigDetails) makeConfig() map[string]interface{} {

	params := map[string]interface{}{}

	for k, v := range m.Options {
		params[k] = v
	}

	for k, v := range m.Forced {
		params[k] = v
	}

	return params
}
