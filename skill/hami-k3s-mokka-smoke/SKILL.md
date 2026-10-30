---
name: hami-k3s-mokka-smoke
description: 在无 NVIDIA GPU 的隔离 k3d/K3s 集群中，用官方 Mokka 和真实开源 HAMi 组件准备模拟 GPU 环境；按需验证设备注册、调度和 Allocate。默认只准备并交接环境，不代表真实 GPU 或 CUDA 验收。
---

# K3s + Mokka + HAMi 模拟环境

默认只准备环境并交接，不自动创建测试 Pod、注入故障、重启插件或删除集群。

| 需求 | 读取顺序 |
| --- | --- |
| 新建隔离环境 | [环境准备](references/environment.md) |
| 在已有环境中验证功能 | [可选回归](references/validation.md)；若尚未获得环境信息，先读[环境准备](references/environment.md)的交接要求 |
| 结束本次自建环境 | [环境准备](references/environment.md)的「按需清理」 |

只操作本次任务获准创建或明确交接的独立集群；每次使用唯一集群名、专用 kubeconfig 和固定镜像引用。Mokka 提供模拟 NVML、驱动文件和设备节点；HAMi scheduler 与 NVIDIA device-plugin 必须是实际组件。区分环境就绪、设备注册、调度、Allocate 和容器内可见性，分别留证。模拟结果不能证明真实 GPU 的 CUDA 执行、显存或算力隔离、性能、MIG，也不能作为生产验收。
