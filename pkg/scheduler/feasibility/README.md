# Complete occupancy Filter extension (local alpha experiment)

The existing Filter endpoint recognizes the optional `simulation` object with
version `experimental-full-occupancy-v1alpha1`. This is a local experimental
contract, not an accepted Kubernetes or upstream API. It carries complete Nodes,
the candidate Pod, managed resource names and the full resident Pod list for
each candidate node. It has no dependency on an autoscaler's implementation,
snapshot operations, lifecycle labels, or call order.

Each resident contains its full `pod` and an `allocation` object whose sole
field is `mode`:

- `preserve`: HAMi parses its own allocation annotations on the Pod. An
  interested Pod must be bound to the target node in both `spec.nodeName` and
  `hami.io/vgpu-node`, and must contain `hami.io/vgpu-devices-allocated`.
  The recorded devices, per-container usage and user constraints are validated
  against the supplied inventory without repacking that Pod. Missing records,
  mismatched bindings or invalid assignments are errors. A Pod without NVIDIA
  demand and without NVIDIA allocation metadata needs no NVIDIA assignment.
- `allocate`: HAMi ignores its generated assignment annotations on a local Pod
  copy, retaining user constraints such as allowed/excluded UUIDs. This also
  applies to the candidate Pod. Input Pods are never modified.

There is no caller-generated allocation encoding or separate allocation payload.
The evaluator owns metadata parsing. The caller is responsible for truthful,
complete occupancy and for choosing preserve versus allocate from its own state.

The implementation currently supports NVIDIA hami-core only. It validates
declared resource names and raw resource demand, then evaluates preserved
residents, other residents in stable UID order, and finally the candidate.
It uses the NVIDIA allocator and the scheduler's init/sidecar peak-usage rules
on request-local state. Ordering can conservatively reject feasible alternative
packings. No live scheduling cache or namespace quota is consulted or modified.
Namespace quotas and other nonlocal policies are outside this contract.

Missing or unsupported node inventory rejects that node. Invalid occupancy,
unknown versions, missing preservation metadata or unrepresentable resource
demand return an error. MIG and other device backends remain unsupported.
The response acknowledges the protocol version and raw request digest only
after successful evaluation. Requests without the extension retain the legacy
Filter path; neither a new endpoint nor a server-side session is introduced.

Tests cover the real allocator and HTTP integration with the caller's basic and
delta snapshots, including rollback, templates and cumulative occupancy. They
do not prove driver allocation, workload execution or real cloud scaling.
