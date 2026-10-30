---
name: hami-k3s-mokka-smoke
description: 在没有 NVIDIA GPU 的主机上，用隔离的 k3d/K3s 集群、官方 Mokka 和真实开源 HAMi 组件准备模拟 GPU 环境；按任务需要验证设备注册、调度、Allocate 或容量行为。
---

# 在模拟 GPU 环境中运行开源 HAMi

先确定交付目标：仅准备环境，还是验证指定行为。用户未要求功能验证时，完成组件就绪与环境交接即停止；不要自行创建测试 Pod、注入故障、重启插件或销毁集群。

按当前任务读取所需内容：

- 新建或交接环境：读[环境准备](references/environment.md)。用户未指定版本且需要可复现起点时，再读[已测版本示例](references/preset-v2.10.0.md)；示例不是所有环境的兼容承诺。
- 使用已交接环境验证指定行为：读[功能验证](references/validation.md)；缺少环境信息时，再读[环境准备](references/environment.md)的交接要求。
- 清理本次环境：读[环境准备](references/environment.md)的清理条件。没有明确归属和停用条件时不要删除。

交付每一层的实际证据、未验证项和环境归属。报告与运行日志保存在任务工作目录，不写入本 Skill；后续使用者需要复现时，从版本选择和环境检查重新开始。

只操作本次任务获准创建或明确交接的隔离集群，使用专用 kubeconfig，不切换默认 context，也不修改共享集群。Mokka 只模拟 NVML、驱动文件和设备节点；HAMi scheduler 与 NVIDIA device-plugin 必须运行真实组件。把「环境就绪」「设备注册」「调度」「Allocate」「容器可见」分别判断。模拟结果不能证明 CUDA kernel、真实显存或算力隔离、性能、MIG，也不是生产验收。
