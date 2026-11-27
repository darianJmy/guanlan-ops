package handlers

import (
	"encoding/json"
	"errors"
	"strings"

	"guanlan/internal/models"

	"k8s.io/client-go/tools/clientcmd"
)

// Import resolves a context into the same fields as a manually configured connection.
func importKubeconfig(ds *models.Datasource) error {
	var input struct {
		Kubeconfig string `json:"kubeconfig"`
		Context    string `json:"context"`
	}
	if len(ds.AuthConfig) > 1<<20 || json.Unmarshal([]byte(ds.AuthConfig), &input) != nil || strings.TrimSpace(input.Kubeconfig) == "" {
		return errors.New("请粘贴有效的 kubeconfig，认证配置不能超过 1 MiB")
	}
	config, err := clientcmd.Load([]byte(input.Kubeconfig))
	if err != nil {
		return errors.New("kubeconfig 格式不正确")
	}
	selected := strings.TrimSpace(input.Context)
	if selected == "" {
		selected = config.CurrentContext
	}
	if selected == "" && len(config.Contexts) == 1 {
		for name := range config.Contexts {
			selected = name
		}
	}
	context := config.Contexts[selected]
	if context == nil {
		return errors.New("请填写 kubeconfig 中有效的 Context")
	}
	cluster, user := config.Clusters[context.Cluster], config.AuthInfos[context.AuthInfo]
	if cluster == nil || user == nil {
		return errors.New("所选 Context 缺少集群或用户配置")
	}
	if cluster.CertificateAuthority != "" || user.TokenFile != "" || user.ClientCertificate != "" || user.ClientKey != "" || user.Exec != nil || user.AuthProvider != nil {
		return errors.New("所选 Context 需要使用内嵌 Token 或客户端证书")
	}
	if cluster.ProxyURL != "" || user.Impersonate != "" || user.ImpersonateUID != "" || len(user.ImpersonateGroups) != 0 || len(user.ImpersonateUserExtra) != 0 || user.Username != "" || user.Password != "" {
		return errors.New("所选 Context 使用了暂不支持的代理、身份模拟或用户名密码配置")
	}
	auth := DatasourceAuthConfig{Token: strings.TrimSpace(user.Token), ClientCertificate: string(user.ClientCertificateData), ClientKey: string(user.ClientKeyData)}
	switch {
	case auth.Token != "" && auth.ClientCertificate == "" && auth.ClientKey == "":
		ds.AuthType = "bearer"
	case auth.Token == "" && auth.ClientCertificate != "" && auth.ClientKey != "":
		ds.AuthType = "client_certificate"
	default:
		return errors.New("所选 Context 需要一组内嵌 Token 或客户端证书与私钥")
	}
	options := map[string]json.RawMessage{}
	if ds.Config != "" && (json.Unmarshal([]byte(ds.Config), &options) != nil || options == nil) {
		return errors.New("Kubernetes 配置格式不正确")
	}
	options["kubernetes"], _ = json.Marshal(kubernetesConnectionConfig{
		Namespace: context.Namespace, CACertificate: string(cluster.CertificateAuthorityData),
		InsecureSkipVerify: cluster.InsecureSkipTLSVerify, TLSServerName: cluster.TLSServerName,
	})
	rawAuth, _ := json.Marshal(auth)
	rawConfig, _ := json.Marshal(options)
	ds.URL = strings.TrimRight(strings.TrimSpace(cluster.Server), "/")
	ds.AuthConfig, ds.Config = string(rawAuth), string(rawConfig)
	return nil
}
