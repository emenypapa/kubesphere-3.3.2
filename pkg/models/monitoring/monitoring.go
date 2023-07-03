/*
Copyright 2019 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package monitoring

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"math"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"sigs.k8s.io/application/api/v1beta1"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"

	"kubesphere.io/api/iam/v1alpha2"

	"github.com/parnurzeal/gorequest"
	"kubesphere.io/kubesphere/pkg/apiserver/query"
	ksinformers "kubesphere.io/kubesphere/pkg/client/informers/externalversions"
	"kubesphere.io/kubesphere/pkg/constants"
	"kubesphere.io/kubesphere/pkg/informers"
	"kubesphere.io/kubesphere/pkg/models/monitoring/expressions"
	"kubesphere.io/kubesphere/pkg/models/openpitrix"
	resourcev1alpha3 "kubesphere.io/kubesphere/pkg/models/resources/v1alpha3/resource"
	"kubesphere.io/kubesphere/pkg/server/errors"
	"kubesphere.io/kubesphere/pkg/server/params"
	meteringclient "kubesphere.io/kubesphere/pkg/simple/client/metering"
	"kubesphere.io/kubesphere/pkg/simple/client/monitoring"
)

type MonitoringOperator interface {
	GetMetric(expr, namespace string, time time.Time) (monitoring.Metric, error)
	GetMetricOverTime(expr, namespace string, start, end time.Time, step time.Duration) (monitoring.Metric, error)
	GetNamedMetrics(metrics []string, time time.Time, opt monitoring.QueryOption) Metrics
	GetNamedMetricsOverTime(metrics []string, start, end time.Time, step time.Duration, opt monitoring.QueryOption) Metrics
	GetMetadata(namespace string) Metadata
	GetMetricLabelSet(metric, namespace string, start, end time.Time) MetricLabelSet

	// TODO: expose KubeSphere self metrics in Prometheus format
	GetKubeSphereStats() Metrics
	GetWorkspaceStats(workspace string) Metrics

	// meter
	GetNamedMetersOverTime(metrics []string, start, end time.Time, step time.Duration, opt monitoring.QueryOption, priceInfo meteringclient.PriceInfo) (Metrics, error)
	GetNamedMeters(metrics []string, time time.Time, opt monitoring.QueryOption, priceInfo meteringclient.PriceInfo) (Metrics, error)
	GetAppWorkloads(ns string, apps []string) map[string][]string
	GetSerivePodsMap(ns string, services []string) map[string][]string
}

type monitoringOperator struct {
	prometheus     monitoring.Interface
	metricsserver  monitoring.Interface
	k8s            kubernetes.Interface
	ks             ksinformers.SharedInformerFactory
	op             openpitrix.Interface
	resourceGetter *resourcev1alpha3.ResourceGetter
}

func NewMonitoringOperator(monitoringClient monitoring.Interface, metricsClient monitoring.Interface, k8s kubernetes.Interface, factory informers.InformerFactory, resourceGetter *resourcev1alpha3.ResourceGetter, op openpitrix.Interface) MonitoringOperator {
	return &monitoringOperator{
		prometheus:     monitoringClient,
		metricsserver:  metricsClient,
		k8s:            k8s,
		ks:             factory.KubeSphereSharedInformerFactory(),
		resourceGetter: resourceGetter,
		op:             op,
	}
}

func (mo monitoringOperator) GetMetric(expr, namespace string, time time.Time) (monitoring.Metric, error) {
	if namespace != "" {
		// Different monitoring backend implementations have different ways to enforce namespace isolation.
		// Each implementation should register itself to `ReplaceNamespaceFns` during init().
		// We hard code "prometheus" here because we only support this datasource so far.
		// In the future, maybe the value should be returned from a method like `mo.c.GetMonitoringServiceName()`.
		var err error
		expr, err = expressions.ReplaceNamespaceFns["prometheus"](expr, namespace)
		if err != nil {
			return monitoring.Metric{}, err
		}
	}
	return mo.prometheus.GetMetric(expr, time), nil
}

func (mo monitoringOperator) GetMetricOverTime(expr, namespace string, start, end time.Time, step time.Duration) (monitoring.Metric, error) {
	if namespace != "" {
		// Different monitoring backend implementations have different ways to enforce namespace isolation.
		// Each implementation should register itself to `ReplaceNamespaceFns` during init().
		// We hard code "prometheus" here because we only support this datasource so far.
		// In the future, maybe the value should be returned from a method like `mo.c.GetMonitoringServiceName()`.
		var err error
		expr, err = expressions.ReplaceNamespaceFns["prometheus"](expr, namespace)
		if err != nil {
			return monitoring.Metric{}, err
		}
	}
	return mo.prometheus.GetMetricOverTime(expr, start, end, step), nil
}

func (mo monitoringOperator) GetNamedMetrics(metrics []string, t time.Time, opt monitoring.QueryOption) Metrics {
	ress := mo.prometheus.GetNamedMetrics(metrics, t, opt)

	opts := &monitoring.QueryOptions{}
	opt.Apply(opts)

	var isNodeRankingQuery bool
	if opts.QueryType == "rank" {
		isNodeRankingQuery = true
	}

	if mo.metricsserver != nil {
		//Merge edge node metrics data
		edgeMetrics := make(map[string]monitoring.MetricData)

		for i, ressMetric := range ress {
			metricName := ressMetric.MetricName
			ressMetricValues := ressMetric.MetricData.MetricValues
			if len(ressMetricValues) == 0 || isNodeRankingQuery {
				// this metric has no prometheus metrics data or the request need to list all nodes metrics
				if len(edgeMetrics) == 0 {
					// start to request monintoring metricsApi data
					mr := mo.metricsserver.GetNamedMetrics(metrics, t, opt)
					for _, mrMetric := range mr {
						edgeMetrics[mrMetric.MetricName] = mrMetric.MetricData
					}
				}
				if val, ok := edgeMetrics[metricName]; ok {
					ress[i].MetricData.MetricValues = append(ress[i].MetricData.MetricValues, val.MetricValues...)
				}
			}

		}
	}

	var fpgaCpuTotalNum, fpgaCpuUsedNum, fpgaCpuFreeNum, fpgaMemTotalNum, fpgaMemUsedNum, fpgaMemFreeNum, tpuMemTotalNum, tpuMemUsedNum, tpuMemFreeNum, tpuUsageNum int64

	var nodeTpuMemTotalMetrics, nodeTpuMemUsedMetrics, nodeTpuMemUsageMetrics, nodeFpgaMemTotalMetrics, nodeFpgaMemUsedMetrics, nodeFpgaCpuTotalMetrics, nodeFpgaCpuUsedMetric []monitoring.MetricValue

	nodes, err := mo.listEdgeNodes()
	if err != nil {
		klog.Errorf("List edge nodes error %v\n", err)
	} else {
		for _, node := range nodes.Items {
			var nodeIP string
			var nodeName string
			for _, address := range node.Status.Addresses {
				if address.Type == v1.NodeInternalIP {
					nodeIP = address.Address
				}
			}

			nodeTpuMemTotalMateDate := make(map[string]string)
			nodeTpuMemTotalMateDate["__name__"] = "node:node_tpu_mem_total:"
			nodeTpuMemTotalMateDate["host_ip"] = nodeIP
			nodeTpuMemTotalMateDate["node"] = nodeName

			nodeTpuMemUsedMateDate := make(map[string]string)
			nodeTpuMemUsedMateDate["__name__"] = "node:node_tpu_mem_used:"
			nodeTpuMemUsedMateDate["host_ip"] = nodeIP
			nodeTpuMemUsedMateDate["node"] = nodeName

			nodeTpuMemUsageMateDate := make(map[string]string)
			nodeTpuMemUsageMateDate["__name__"] = "node:node_tpu_mem_usage:"
			nodeTpuMemUsageMateDate["host_ip"] = nodeIP
			nodeTpuMemUsageMateDate["node"] = nodeName

			nodeFpgaMemTotalMateDate := make(map[string]string)
			nodeFpgaMemTotalMateDate["__name__"] = "node:node_fpga_mem_total:"
			nodeFpgaMemTotalMateDate["host_ip"] = nodeIP
			nodeFpgaMemTotalMateDate["node"] = nodeName

			nodeFpgaMemUsedMateDate := make(map[string]string)
			nodeFpgaMemUsedMateDate["__name__"] = "node:node_fpga_mem_used:"
			nodeFpgaMemUsedMateDate["host_ip"] = nodeIP
			nodeFpgaMemUsedMateDate["node"] = nodeName

			nodeFpgaCpuTotalMateDate := make(map[string]string)
			nodeFpgaCpuTotalMateDate["__name__"] = "node:node_fpga_cpu_total:"
			nodeFpgaCpuTotalMateDate["host_ip"] = nodeIP
			nodeFpgaCpuTotalMateDate["node"] = nodeName

			nodeFpgaCpuUsedMateDate := make(map[string]string)
			nodeFpgaCpuUsedMateDate["__name__"] = "node:node_fpga_cpu_used:"
			nodeFpgaCpuUsedMateDate["host_ip"] = nodeIP
			nodeFpgaCpuUsedMateDate["node"] = nodeName

			err, tpuMemRes := RpcTpuMemAnalysis(nodeIP)
			if err != nil {
				klog.Errorf("RpcTpuMemAnalysis error %v\n", err)
			}
			tpuMemTotalNum += tpuMemRes.Data.TotalMemSize
			tpuMemUsedNum += tpuMemRes.Data.UsedMemSize
			tpuMemFreeNum += tpuMemRes.Data.FreeMemSize

			err, tpuUsageRes := RpcTpuUsageAnalysis(nodeIP)
			if err != nil {
				klog.Errorf("RpcTpuUsageAnalysis error %v\n", err)
			}
			tpuUsageNum += int64(tpuUsageRes.Data)

			nodeTpuMemTotalMetricValue := monitoring.MetricValue{
				Metadata: nodeTpuMemTotalMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(tpuMemRes.Data.TotalMemSize)},
			}
			nodeTpuMemTotalMetrics = append(nodeTpuMemTotalMetrics, nodeTpuMemTotalMetricValue)
			nodeTpuMemUsedMetricValue := monitoring.MetricValue{
				Metadata: nodeTpuMemUsedMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(tpuMemRes.Data.UsedMemSize)},
			}
			nodeTpuMemUsedMetrics = append(nodeTpuMemUsedMetrics, nodeTpuMemUsedMetricValue)
			nodeTpuMemUsageMetricValue := monitoring.MetricValue{
				Metadata: nodeTpuMemUsageMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageRes.Data)},
			}
			nodeTpuMemUsageMetrics = append(nodeTpuMemUsageMetrics, nodeTpuMemUsageMetricValue)
			err, fpgaDetailsRes := RpcFpgaDetailsAnalysis(nodeIP)
			if err != nil {
				klog.Errorf("RpcFpgaDetailsAnalysis error %v\n", err)
			}

			fpgaCpuTotalNum += fpgaDetailsRes.Data.TotalCpuSize
			fpgaCpuUsedNum += fpgaDetailsRes.Data.UsedCpuSize
			fpgaCpuFreeNum += fpgaDetailsRes.Data.FreeCpuSize
			fpgaMemTotalNum += fpgaDetailsRes.Data.TotalMemSize
			fpgaMemUsedNum += fpgaDetailsRes.Data.UsedMemSize
			fpgaMemFreeNum += fpgaDetailsRes.Data.FreeMemSize

			nodeFpgaMemTotalMetricValue := monitoring.MetricValue{
				Metadata: nodeFpgaMemTotalMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(fpgaDetailsRes.Data.TotalMemSize)},
			}
			nodeFpgaMemTotalMetrics = append(nodeFpgaMemTotalMetrics, nodeFpgaMemTotalMetricValue)
			nodeFpgaMemUsedMetricValue := monitoring.MetricValue{
				Metadata: nodeFpgaMemUsedMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(fpgaDetailsRes.Data.UsedMemSize)},
			}
			nodeFpgaMemUsedMetrics = append(nodeFpgaMemUsedMetrics, nodeFpgaMemUsedMetricValue)
			nodeFpgaCpuTotalMetricValue := monitoring.MetricValue{
				Metadata: nodeFpgaCpuTotalMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(fpgaDetailsRes.Data.TotalCpuSize)},
			}
			nodeFpgaCpuTotalMetrics = append(nodeFpgaCpuTotalMetrics, nodeFpgaCpuTotalMetricValue)
			nodeFpgaCpuUsedMetricValue := monitoring.MetricValue{
				Metadata: nodeFpgaCpuUsedMateDate,
				Sample:   &monitoring.Point{float64(time.Now().Unix()), float64(fpgaDetailsRes.Data.UsedCpuSize)},
			}
			nodeFpgaCpuUsedMetric = append(nodeFpgaCpuUsedMetric, nodeFpgaCpuUsedMetricValue)
		}

		if tpuUsageNum != 0 {
			tpuUsageNum = tpuUsageNum / int64(len(nodes.Items))
		}

	}

	//metricValue := monitoring.MetricValue{
	//	//Metadata: make(map[string]string),
	//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
	//}

	for i := 0; i < len(ress); i++ {
		switch ress[i].MetricName {
		case "node_tpu_mem_total":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeTpuMemTotalMetrics...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "node_tpu_mem_used":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeTpuMemUsedMetrics...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "node_tpu_mem_usage":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeTpuMemUsageMetrics...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "node_fpga_mem_total":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeFpgaMemTotalMetrics...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "node_fpga_mem_used":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeFpgaMemUsedMetrics...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "node_fpga_cpu_total":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeFpgaCpuTotalMetrics...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "node_fpga_cpu_used":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			//metricValue := monitoring.MetricValue{
			//	Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			//}
			res.MetricValues = append(res.MetricValues, nodeFpgaCpuUsedMetric...)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_tpu_usage":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuUsageNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_tpu_mem_total":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuMemTotalNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_tpu_mem_used":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuMemUsedNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_tpu_mem_free":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(tpuMemFreeNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_fpga_cpu_total":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(fpgaCpuTotalNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_fpga_cpu_used":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(fpgaCpuUsedNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_fpga_cpu_free":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(fpgaCpuFreeNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_fpga_mem_total":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(fpgaMemTotalNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_fpga_mem_used":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(fpgaMemUsedNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		case "cluster_fpga_mem_free":
			res := monitoring.MetricData{MetricType: monitoring.MetricTypeVector}
			metricValue := monitoring.MetricValue{
				Sample: &monitoring.Point{float64(time.Now().Unix()), float64(fpgaMemFreeNum)},
			}
			res.MetricValues = append(res.MetricValues, metricValue)
			ress[i].MetricData = res
			ress[i].Error = ""
		}
	}

	return Metrics{Results: ress}
}

func (mo monitoringOperator) GetNodeIPs(nodes *v1.NodeList) []string {
	var nodeIPs []string
	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == v1.NodeInternalIP {
				nodeIPs = append(nodeIPs, address.Address)
			}
		}
	}
	return nodeIPs
}

func (mo monitoringOperator) listEdgeNodes() (*v1.NodeList, error) {
	nodeClient := mo.k8s.CoreV1()

	nodeList, err := nodeClient.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nodeList, err
	}

	return nodeList, nil
}

type TpuMemRpcResponse struct {
	Code int32          `json:"code"`
	Msg  string         `json:"msg"`
	Time time.Time      `json:"time"`
	Data TpuMemAnalysis `json:"data"`
}

type TpuUsageRpcResponse struct {
	Code int32     `json:"code"`
	Msg  string    `json:"msg"`
	Time time.Time `json:"time"`
	Data int       `json:"data"`
}

type FpgaDetailsRpcResponse struct {
	Code int32               `json:"code"`
	Msg  string              `json:"msg"`
	Time time.Time           `json:"time"`
	Data FpgaDetailsAnalysis `json:"data"`
}

type FpgaDetailsAnalysis struct {
	TotalCpuSize int64 `json:"total_cpu_size"`
	UsedCpuSize  int64 `json:"used_cpu_size"`
	FreeCpuSize  int64 `json:"free_cpu_size"`
	TotalMemSize int64 `json:"total_mem_size"`
	UsedMemSize  int64 `json:"used_mem_size"`
	FreeMemSize  int64 `json:"free_mem_size"`
}

type TpuMemAnalysis struct {
	TotalMemSize int64 `json:"total_mem_size"`
	UsedMemSize  int64 `json:"used_mem_size"`
	FreeMemSize  int64 `json:"free_mem_size"`
}

func RpcTpuMemAnalysis(nodeIp string) (err error, res TpuMemRpcResponse) {
	rawURL := "http://" + nodeIp + ":32112/metrics/tpu_mem"
	request := gorequest.New()
	request.Get(rawURL)

	//logger.Infof(ctx, " rawURL=[%+v] paramStr = [%+v]", rawURL, requestParam)
	//request.Patch(rawURL).Header.Add("Authorization", token)
	_, _, errs := request.EndStruct(&res)
	//logger.Infof(ctx, " errs=[%+v] urlResp = [%+v]", errs, res)
	if len(errs) > 0 {
		err = errs[0]
		return
	}
	return
}

func RpcTpuUsageAnalysis(nodeIp string) (err error, res TpuUsageRpcResponse) {
	rawURL := "http://" + nodeIp + ":32112/metrics/tpu_usage"
	request := gorequest.New()
	request.Get(rawURL)

	//logger.Infof(ctx, " rawURL=[%+v] paramStr = [%+v]", rawURL, requestParam)
	//request.Patch(rawURL).Header.Add("Authorization", token)
	_, _, errs := request.EndStruct(&res)
	//logger.Infof(ctx, " errs=[%+v] urlResp = [%+v]", errs, res)
	if len(errs) > 0 {
		err = errs[0]
		return
	}
	return
}

func RpcFpgaDetailsAnalysis(nodeIp string) (err error, res FpgaDetailsRpcResponse) {
	rawURL := "http://" + nodeIp + ":32112/metrics/fpga_details"
	request := gorequest.New()
	request.Get(rawURL)

	//logger.Infof(ctx, " rawURL=[%+v] paramStr = [%+v]", rawURL, requestParam)
	//request.Patch(rawURL).Header.Add("Authorization", token)
	_, _, errs := request.EndStruct(&res)
	//logger.Infof(ctx, " errs=[%+v] urlResp = [%+v]", errs, res)
	if len(errs) > 0 {
		err = errs[0]
		return
	}
	return
}

func (mo monitoringOperator) GetNamedMetricsOverTime(metrics []string, start, end time.Time, step time.Duration, opt monitoring.QueryOption) Metrics {
	ress := mo.prometheus.GetNamedMetricsOverTime(metrics, start, end, step, opt)

	if mo.metricsserver != nil {

		//Merge edge node metrics data
		edgeMetrics := make(map[string]monitoring.MetricData)

		for i, ressMetric := range ress {
			metricName := ressMetric.MetricName
			ressMetricValues := ressMetric.MetricData.MetricValues
			if len(ressMetricValues) == 0 {
				// this metric has no prometheus metrics data
				if len(edgeMetrics) == 0 {
					// start to request monintoring metricsApi data
					mr := mo.metricsserver.GetNamedMetricsOverTime(metrics, start, end, step, opt)
					for _, mrMetric := range mr {
						edgeMetrics[mrMetric.MetricName] = mrMetric.MetricData
					}
				}
				if val, ok := edgeMetrics[metricName]; ok {
					ress[i].MetricData.MetricValues = append(ress[i].MetricData.MetricValues, val.MetricValues...)
				}
			}
		}
	}

	return Metrics{Results: ress}
}

func (mo monitoringOperator) GetMetadata(namespace string) Metadata {
	data := mo.prometheus.GetMetadata(namespace)
	return Metadata{Data: data}
}

func (mo monitoringOperator) GetMetricLabelSet(metric, namespace string, start, end time.Time) MetricLabelSet {
	var expr = metric
	var err error
	if namespace != "" {
		// Different monitoring backend implementations have different ways to enforce namespace isolation.
		// Each implementation should register itself to `ReplaceNamespaceFns` during init().
		// We hard code "prometheus" here because we only support this datasource so far.
		// In the future, maybe the value should be returned from a method like `mo.c.GetMonitoringServiceName()`.
		expr, err = expressions.ReplaceNamespaceFns["prometheus"](metric, namespace)
		if err != nil {
			klog.Error(err)
			return MetricLabelSet{}
		}
	}
	data := mo.prometheus.GetMetricLabelSet(expr, start, end)
	return MetricLabelSet{Data: data}
}

func (mo monitoringOperator) GetKubeSphereStats() Metrics {
	var res Metrics
	now := float64(time.Now().Unix())

	clusterList, err := mo.ks.Cluster().V1alpha1().Clusters().Lister().List(labels.Everything())
	clusterTotal := len(clusterList)
	if clusterTotal == 0 {
		clusterTotal = 1
	}
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: KubeSphereClusterCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: KubeSphereClusterCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(clusterTotal)},
					},
				},
			},
		})
	}

	wkList, err := mo.ks.Tenant().V1alpha2().WorkspaceTemplates().Lister().List(labels.Everything())
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: KubeSphereWorkspaceCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: KubeSphereWorkspaceCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(len(wkList))},
					},
				},
			},
		})
	}

	usrList, err := mo.ks.Iam().V1alpha2().Users().Lister().List(labels.Everything())
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: KubeSphereUserCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: KubeSphereUserCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(len(usrList))},
					},
				},
			},
		})
	}

	cond := &params.Conditions{
		Match: map[string]string{
			openpitrix.Status: openpitrix.StatusActive,
			openpitrix.RepoId: openpitrix.BuiltinRepoId,
		},
	}
	if mo.op != nil {
		tmpl, err := mo.op.ListApps(cond, "", false, 0, 0)
		if err != nil {
			res.Results = append(res.Results, monitoring.Metric{
				MetricName: KubeSphereAppTmplCount,
				Error:      err.Error(),
			})
		} else {
			res.Results = append(res.Results, monitoring.Metric{
				MetricName: KubeSphereAppTmplCount,
				MetricData: monitoring.MetricData{
					MetricType: monitoring.MetricTypeVector,
					MetricValues: []monitoring.MetricValue{
						{
							Sample: &monitoring.Point{now, float64(tmpl.TotalCount)},
						},
					},
				},
			})
		}
	}

	return res
}

func (mo monitoringOperator) GetWorkspaceStats(workspace string) Metrics {
	var res Metrics
	now := float64(time.Now().Unix())

	selector := labels.SelectorFromSet(labels.Set{constants.WorkspaceLabelKey: workspace})
	opt := metav1.ListOptions{LabelSelector: selector.String()}

	nsList, err := mo.k8s.CoreV1().Namespaces().List(context.Background(), opt)
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceNamespaceCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceNamespaceCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(len(nsList.Items))},
					},
				},
			},
		})
	}

	devopsList, err := mo.ks.Devops().V1alpha3().DevOpsProjects().Lister().List(selector)
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceDevopsCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceDevopsCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(len(devopsList))},
					},
				},
			},
		})
	}

	r, _ := labels.NewRequirement(v1alpha2.UserReferenceLabel, selection.Exists, nil)
	memberSelector := selector.DeepCopySelector().Add(*r)
	memberList, err := mo.ks.Iam().V1alpha2().WorkspaceRoleBindings().Lister().List(memberSelector)
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceMemberCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceMemberCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(len(memberList))},
					},
				},
			},
		})
	}

	roleList, err := mo.ks.Iam().V1alpha2().WorkspaceRoles().Lister().List(selector)
	if err != nil {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceRoleCount,
			Error:      err.Error(),
		})
	} else {
		res.Results = append(res.Results, monitoring.Metric{
			MetricName: WorkspaceRoleCount,
			MetricData: monitoring.MetricData{
				MetricType: monitoring.MetricTypeVector,
				MetricValues: []monitoring.MetricValue{
					{
						Sample: &monitoring.Point{now, float64(len(roleList))},
					},
				},
			},
		})
	}

	return res
}

/*
	meter related methods
*/

func (mo monitoringOperator) getNamedMetersWithHourInterval(meters []string, t time.Time, opt monitoring.QueryOption) Metrics {

	var opts []monitoring.QueryOption

	opts = append(opts, opt)
	opts = append(opts, monitoring.MeterOption{
		Step: 1 * time.Hour,
	})

	ress := mo.prometheus.GetNamedMeters(meters, t, opts)

	return Metrics{Results: ress}
}

func generateScalingFactorMap(step time.Duration) map[string]float64 {
	scalingMap := make(map[string]float64)

	for k := range MeterResourceMap {
		scalingMap[k] = step.Hours()
	}
	return scalingMap
}

func (mo monitoringOperator) GetNamedMetersOverTime(meters []string, start, end time.Time, step time.Duration, opt monitoring.QueryOption, priceInfo meteringclient.PriceInfo) (metrics Metrics, err error) {

	if step.Hours() < 1 {
		klog.Warning("step should be longer than one hour")
		step = 1 * time.Hour
	}
	if end.Sub(start).Hours() > 30*24 {
		if step.Hours() < 24 {
			err = errors.New("step should be larger than 24 hours")
			return
		}
	}
	if math.Mod(step.Hours(), 1.0) > 0 {
		err = errors.New("step should be integer hours")
		return
	}

	// query time range: (start, end], so here we need to exclude start itself.
	if start.Add(time.Hour).After(end) {
		start = end
	} else {
		start = start.Add(time.Hour)
	}

	var opts []monitoring.QueryOption

	opts = append(opts, opt)
	opts = append(opts, monitoring.MeterOption{
		Start: start,
		End:   end,
		Step:  time.Hour,
	})

	ress := mo.prometheus.GetNamedMetersOverTime(meters, start, end, time.Hour, opts)
	sMap := generateScalingFactorMap(step)

	for i := range ress {
		ress[i].MetricData = updateMetricStatData(ress[i], sMap, priceInfo)
	}

	return Metrics{Results: ress}, nil
}

func (mo monitoringOperator) GetNamedMeters(meters []string, time time.Time, opt monitoring.QueryOption, priceInfo meteringclient.PriceInfo) (Metrics, error) {

	metersPerHour := mo.getNamedMetersWithHourInterval(meters, time, opt)

	for metricIndex := range metersPerHour.Results {

		res := metersPerHour.Results[metricIndex]

		metersPerHour.Results[metricIndex].MetricData = updateMetricStatData(res, nil, priceInfo)
	}

	return metersPerHour, nil
}

func (mo monitoringOperator) GetAppWorkloads(ns string, apps []string) map[string][]string {

	componentsMap := make(map[string][]string)
	applicationList := []*appv1beta1.Application{}

	result, err := mo.resourceGetter.List("applications", ns, query.New())
	if err != nil {
		klog.Error(err)
		return nil
	}

	for _, obj := range result.Items {
		app, ok := obj.(*appv1beta1.Application)
		if !ok {
			continue
		}

		applicationList = append(applicationList, app)
	}

	getAppFullName := func(appObject *v1beta1.Application) (name string) {
		name = appObject.Labels[constants.ApplicationName]
		if appObject.Labels[constants.ApplicationVersion] != "" {
			name += fmt.Sprintf(":%v", appObject.Labels[constants.ApplicationVersion])
		}
		return
	}

	appFilter := func(appObject *v1beta1.Application) bool {

		for _, app := range apps {
			var applicationName, applicationVersion string
			tmp := strings.Split(app, ":")

			if len(tmp) >= 1 {
				applicationName = tmp[0]
			}
			if len(tmp) == 2 {
				applicationVersion = tmp[1]
			}

			if applicationName != "" && appObject.Labels[constants.ApplicationName] != applicationName {
				return false
			}
			if applicationVersion != "" && appObject.Labels[constants.ApplicationVersion] != applicationVersion {
				return false
			}
			return true
		}

		return true
	}

	for _, appObj := range applicationList {
		if appFilter(appObj) {
			for _, com := range appObj.Status.ComponentList.Objects {
				kind := strings.Title(com.Kind)
				name := com.Name
				componentsMap[getAppFullName((appObj))] = append(componentsMap[getAppFullName(appObj)], kind+":"+name)
			}
		}
	}

	return componentsMap
}

func (mo monitoringOperator) getApplicationPVCs(appObject *v1beta1.Application) []string {

	var pvcList []string

	ns := appObject.Namespace
	for _, com := range appObject.Status.ComponentList.Objects {

		switch strings.Title(com.Kind) {
		case "Deployment":
			deployObj, err := mo.k8s.AppsV1().Deployments(ns).Get(context.Background(), com.Name, metav1.GetOptions{})
			if err != nil {
				klog.Error(err.Error())
				return nil
			}

			for _, vol := range deployObj.Spec.Template.Spec.Volumes {
				pvcList = append(pvcList, vol.PersistentVolumeClaim.ClaimName)
			}
		case "Statefulset":
			stsObj, err := mo.k8s.AppsV1().StatefulSets(ns).Get(context.Background(), com.Name, metav1.GetOptions{})
			if err != nil {
				klog.Error(err.Error())
				return nil
			}
			for _, vol := range stsObj.Spec.Template.Spec.Volumes {
				pvcList = append(pvcList, vol.PersistentVolumeClaim.ClaimName)
			}
		}

	}

	return pvcList

}

func (mo monitoringOperator) GetSerivePodsMap(ns string, services []string) map[string][]string {
	var svcPodsMap = make(map[string][]string)

	for _, svc := range services {
		svcObj, err := mo.k8s.CoreV1().Services(ns).Get(context.Background(), svc, metav1.GetOptions{})
		if err != nil {
			klog.Error(err.Error())
			return svcPodsMap
		}

		svcSelector := svcObj.Spec.Selector
		if len(svcSelector) == 0 {
			continue
		}

		svcLabels := labels.Set{}
		for key, value := range svcSelector {
			svcLabels[key] = value
		}

		selector := labels.SelectorFromSet(svcLabels)
		opt := metav1.ListOptions{LabelSelector: selector.String()}

		podList, err := mo.k8s.CoreV1().Pods(ns).List(context.Background(), opt)
		if err != nil {
			klog.Error(err.Error())
			return svcPodsMap
		}

		for _, pod := range podList.Items {
			svcPodsMap[svc] = append(svcPodsMap[svc], pod.Name)
		}

	}
	return svcPodsMap
}
