export const probeTypes = [
  { value: 'http', group: '应用层', label: 'HTTP', title: '应用请求', description: '接口与网页可用性', hint: '配置请求地址、方法和成功状态码', tag: 'primary', defaultPort: 0 },
  { value: 'icmp', group: '网络层', label: 'ICMP', title: '网络连通', description: '检查主机是否可达', hint: '配置目标主机和 ICMP 发包数量', tag: 'success', defaultPort: 0 },
  { value: 'tcp', group: '网络层', label: 'TCP', title: '端口连通', description: '检查服务端口开放', hint: '配置目标主机和 TCP 端口', tag: 'warning', defaultPort: 80 },
  { value: 'certificate_expiry', group: '安全检测', label: '证书到期', title: '有效期检查', description: '在网站证书到期前告警', hint: '检查网站证书到期时间，无法获取证书时单独显示检测失败', tag: 'warning', defaultPort: 443 },
  { value: 'script', group: '自定义检测', label: '脚本', title: '脚本运行', description: '调用自动化任务检查状态', hint: '选择自动化任务，执行配置由任务统一管理', tag: 'info', defaultPort: 0 },
]

export const probeTypeGroups = ['应用层', '网络层', '安全检测', '自定义检测'].map(label => ({ label, options: probeTypes.filter(item => item.group === label) }))
