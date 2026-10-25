package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/gin-gonic/gin"
)

type kafkaAuthConfig struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Mechanism string `json:"mechanism"`
}

type kafkaTopicInfo struct {
	Name       string `json:"name"`
	Partitions int    `json:"partitions"`
}

type kafkaConsumerGroupLag struct {
	Group         string `json:"group"`
	Topic         string `json:"topic"`
	Partition     int32  `json:"partition"`
	CurrentOffset int64  `json:"current_offset"`
	LogEndOffset  int64  `json:"log_end_offset"`
	Lag           int64  `json:"lag"`
	Metadata      string `json:"metadata,omitempty"`
}

type kafkaConsumerGroupSummary struct {
	Group string `json:"group"`
	State string `json:"state,omitempty"`
	Lag   int64  `json:"lag"`
}

type kafkaLagResult struct {
	Groups []kafkaConsumerGroupSummary `json:"groups"`
	Items  []kafkaConsumerGroupLag     `json:"items"`
}

func KafkaTopics(c *gin.Context) {
	ds, ok := loadKafkaDatasource(c)
	if !ok {
		return
	}
	topics, err := listKafkaTopics(ds)
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{"datasource": ds.Name, "count": len(topics), "topics": topics})
}

func KafkaConsumerLag(c *gin.Context) {
	ds, ok := loadKafkaDatasource(c)
	if !ok {
		return
	}
	group := strings.TrimSpace(c.Query("group"))
	result, err := listKafkaConsumerLag(ds, group, intQueryDefault(c, "limit", 200))
	if err != nil {
		resp.Error(c, http.StatusOK, resp.CodeFailure, err.Error())
		return
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{"datasource": ds.Name, "groups": result.Groups, "items": result.Items})
}

func loadKafkaDatasource(c *gin.Context) (models.Datasource, bool) {
	id, _ := strconv.Atoi(c.Param("id"))
	var ds models.Datasource
	if err := database.DB.Where("id = ? AND status = 1 AND type = ?", id, "kafka").First(&ds).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "Kafka 数据源不存在或未启用")
		return ds, false
	}
	return ds, true
}

func testKafka(dsURL, authType, authConfig string) (bool, string) {
	ds := models.Datasource{URL: dsURL, AuthType: authType, AuthConfig: authConfig}
	client, err := newKafkaClient(ds)
	if err != nil {
		return false, "Kafka 连接失败: " + err.Error()
	}
	defer client.Close()
	brokers := client.Brokers()
	if len(brokers) == 0 {
		return false, "Kafka 未返回 broker metadata"
	}
	return true, fmt.Sprintf("连接成功 (Kafka)，发现 %d 个 broker", len(brokers))
}

func listKafkaTopics(ds models.Datasource) ([]kafkaTopicInfo, error) {
	client, err := newKafkaClient(ds)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	names, err := client.Topics()
	if err != nil {
		return nil, fmt.Errorf("读取 topic 列表失败: %w", err)
	}
	sort.Strings(names)
	topics := make([]kafkaTopicInfo, 0, len(names))
	for _, name := range names {
		partitions, err := client.Partitions(name)
		if err != nil {
			topics = append(topics, kafkaTopicInfo{Name: name})
			continue
		}
		topics = append(topics, kafkaTopicInfo{Name: name, Partitions: len(partitions)})
	}
	return topics, nil
}

func listKafkaConsumerLag(ds models.Datasource, groupFilter string, limit int) (kafkaLagResult, error) {
	admin, err := newKafkaAdmin(ds)
	if err != nil {
		return kafkaLagResult{}, err
	}
	defer admin.Close()

	client, err := newKafkaClient(ds)
	if err != nil {
		return kafkaLagResult{}, err
	}
	defer client.Close()

	groups, err := admin.ListConsumerGroups()
	if err != nil {
		return kafkaLagResult{}, fmt.Errorf("读取 consumer group 失败: %w", err)
	}
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		if groupFilter == "" || group == groupFilter {
			groupNames = append(groupNames, group)
		}
	}
	sort.Strings(groupNames)

	result := kafkaLagResult{Groups: make([]kafkaConsumerGroupSummary, 0, len(groupNames))}
	for _, group := range groupNames {
		offsets, err := admin.ListConsumerGroupOffsets(group, nil)
		if err != nil {
			result.Groups = append(result.Groups, kafkaConsumerGroupSummary{Group: group})
			continue
		}
		var groupLag int64
		for topic, partitions := range offsets.Blocks {
			for partition, block := range partitions {
				if block == nil {
					continue
				}
				endOffset, err := client.GetOffset(topic, partition, sarama.OffsetNewest)
				if err != nil {
					continue
				}
				lag := int64(0)
				if block.Offset >= 0 && endOffset >= block.Offset {
					lag = endOffset - block.Offset
				}
				groupLag += lag
				result.Items = append(result.Items, kafkaConsumerGroupLag{
					Group:         group,
					Topic:         topic,
					Partition:     partition,
					CurrentOffset: block.Offset,
					LogEndOffset:  endOffset,
					Lag:           lag,
					Metadata:      block.Metadata,
				})
			}
		}
		result.Groups = append(result.Groups, kafkaConsumerGroupSummary{Group: group, Lag: groupLag})
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].Lag == result.Items[j].Lag {
			return result.Items[i].Topic < result.Items[j].Topic
		}
		return result.Items[i].Lag > result.Items[j].Lag
	})
	sort.Slice(result.Groups, func(i, j int) bool {
		if result.Groups[i].Lag == result.Groups[j].Lag {
			return result.Groups[i].Group < result.Groups[j].Group
		}
		return result.Groups[i].Lag > result.Groups[j].Lag
	})
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func intQueryDefault(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func newKafkaClient(ds models.Datasource) (sarama.Client, error) {
	return sarama.NewClient(kafkaBrokers(ds.URL), kafkaConfig(ds.AuthType, ds.AuthConfig))
}

func newKafkaAdmin(ds models.Datasource) (sarama.ClusterAdmin, error) {
	return sarama.NewClusterAdmin(kafkaBrokers(ds.URL), kafkaConfig(ds.AuthType, ds.AuthConfig))
}

func kafkaBrokers(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(part, "kafka://"), "tcp://")); value != "" {
			brokers = append(brokers, value)
		}
	}
	return brokers
}

func kafkaConfig(authType, authConfigJSON string) *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_1_0_0
	cfg.ClientID = "aiops-platform"
	cfg.Metadata.Timeout = 6 * time.Second
	cfg.Net.DialTimeout = 6 * time.Second
	cfg.Net.ReadTimeout = 6 * time.Second
	cfg.Net.WriteTimeout = 6 * time.Second

	if authType == "basic" {
		var auth kafkaAuthConfig
		_ = json.Unmarshal([]byte(authConfigJSON), &auth)
		if strings.TrimSpace(auth.Username) != "" || strings.TrimSpace(auth.Password) != "" {
			cfg.Net.SASL.Enable = true
			cfg.Net.SASL.User = strings.TrimSpace(auth.Username)
			cfg.Net.SASL.Password = auth.Password
			cfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}
	return cfg
}
