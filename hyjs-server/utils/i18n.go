package utils

import (
	"path/filepath"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

type I18nClient struct {
	bundle *i18n.Bundle
	lang   string
	accept string
}

var i18nCli *I18nClient

func InitI18n() error {
	i18nCli = new(I18nClient)

	i18nCli.bundle = i18n.NewBundle(language.English)
	i18nCli.bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	matches, err := filepath.Glob("locales/active.*.yaml")
	if err != nil {
		return err
	}

	for _, match := range matches {
		if _, err = i18nCli.bundle.LoadMessageFile(match); err != nil {
			return err
		}
	}

	return nil
}

func GetI18nClient() *I18nClient {
	return i18nCli
}

func (cli *I18nClient) SetLang(lang string) {
	cli.lang = lang
}

func (cli *I18nClient) SetAccept(accept string) {
	cli.accept = accept
}

func (cli *I18nClient) Greet(messageID string, templateData map[string]interface{}) string {
	if cli.lang == "" && cli.accept == "" {
		cli.lang = "en"
	}
	localizer := i18n.NewLocalizer(cli.bundle, cli.lang, cli.accept)
	msg, _ := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	return msg
}
