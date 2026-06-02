package powerscale

import "testing"

func TestBaseLabelsAndPrefixes(t *testing.T) {
	b := baseLabels("clu1", "GUID-123")
	if len(b) != 2 || b[0].Name != "cluster" || b[0].Value != "clu1" || b[1].Name != "cluster_id" {
		t.Fatalf("unexpected base labels: %+v", b)
	}
	if metricPrefix[ObjCluster] != "powerscale_cluster" {
		t.Fatalf("cluster prefix = %q", metricPrefix[ObjCluster])
	}
	if metricPrefix[ObjQuota] != "powerscale_quota" {
		t.Fatalf("quota prefix = %q", metricPrefix[ObjQuota])
	}
}

func TestNodeLabels(t *testing.T) {
	labels := nodeLabels("clu1", "GUID-123", "3")
	// canonical order: cluster, cluster_id, node
	if labels[2].Name != "node" || labels[2].Value != "3" {
		t.Fatalf("node label wrong: %+v", labels)
	}
}

func TestQuotaLabelsCanonicalOrder(t *testing.T) {
	labels := quotaLabels("clu1", "GUID-123", "qid", "/ifs/data/proj", "directory")
	want := []string{"cluster", "cluster_id", "quota_id", "quota_path", "quota_type"}
	for i, w := range want {
		if labels[i].Name != w {
			t.Fatalf("label[%d] = %q, want %q", i, labels[i].Name, w)
		}
	}
}

func TestToSnake(t *testing.T) {
	if toSnake("cluster.cpu.sys.avg") != "cluster_cpu_sys_avg" {
		t.Fatalf("toSnake dotted = %q", toSnake("cluster.cpu.sys.avg"))
	}
}
