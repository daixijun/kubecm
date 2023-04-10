package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"reflect"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"

	apiequality "k8s.io/apimachinery/pkg/api/equality"

	"k8s.io/apimachinery/pkg/util/diff"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	appendRootConfigConflictAlfa = clientcmdapi.Config{
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"black-user": {Token: "black-token"}},
		Clusters: map[string]*clientcmdapi.Cluster{
			"pig-cluster": {Server: "http://pig.org:8080"}},
		Contexts: map[string]*clientcmdapi.Context{
			"root-context": {AuthInfo: "black-user", Cluster: "pig-cluster", Namespace: "saw-ns"}},
	}
	appendConfigAlfa = clientcmdapi.Config{
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"red-user": {Token: "red-token"}},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cow-cluster": {Server: "http://cow.org:8080"}},
		Contexts: map[string]*clientcmdapi.Context{
			"federal-context": {AuthInfo: "red-user", Cluster: "cow-cluster", Namespace: "hammer-ns"}},
	}
	appendMergeConfig = clientcmdapi.Config{
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"black-user": {Token: "black-token"},
			"red-user":   {Token: "red-token"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"pig-cluster": {Server: "http://pig.org:8080"},
			"cow-cluster": {Server: "http://cow.org:8080"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"root-context":    {AuthInfo: "black-user", Cluster: "pig-cluster", Namespace: "saw-ns"},
			"federal-context": {AuthInfo: "red-user", Cluster: "cow-cluster", Namespace: "hammer-ns"},
		},
	}
	wrongRootConfig = clientcmdapi.Config{
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"red-user": {Token: "red-token"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cow-cluster": {Server: "http://cow.org:8080"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"root-context":    {AuthInfo: "black-user", Cluster: "pig-cluster", Namespace: "saw-ns"},
			"federal-context": {AuthInfo: "red-user", Cluster: "cow-cluster", Namespace: "hammer-ns"},
		},
	}
	//wrongFederalConfig = clientcmdapi.Config{
	//	AuthInfos: map[string]*clientcmdapi.AuthInfo{
	//		"black-user": {Token: "black-token"},
	//	},
	//	Clusters: map[string]*clientcmdapi.Cluster{
	//		"pig-cluster": {Server: "http://pig.org:8080"},
	//		"cow-cluster": {Server: "http://cow.org:8080"},
	//	},
	//	Contexts: map[string]*clientcmdapi.Context{
	//		"root-context":    {AuthInfo: "black-user", Cluster: "pig-cluster", Namespace: "saw-ns"},
	//		"federal-context": {AuthInfo: "red-user", Cluster: "cow-cluster", Namespace: "hammer-ns"},
	//	},
	//}
)

func Test_appendConfig(t *testing.T) {
	type args struct {
		c1 *clientcmdapi.Config
		c2 *clientcmdapi.Config
	}
	tests := []struct {
		name string
		args args
		want *clientcmdapi.Config
	}{
		// TODO: Add test cases.
		{"merge", args{&appendRootConfigConflictAlfa, &appendConfigAlfa}, &appendMergeConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendConfig(tt.args.c1, tt.args.c2)
			checkResult(tt.want, got, "", t)
		})
	}
}

func checkResult(want, got *clientcmdapi.Config, wantname string, t *testing.T) {
	testSetNilMapsToEmpties(reflect.ValueOf(&got))
	testSetNilMapsToEmpties(reflect.ValueOf(&want))
	testClearLocationOfOrigin(got)

	if !apiequality.Semantic.DeepEqual(want, got) {
		t.Errorf("diff: %v", diff.ObjectDiff(want, got))
		t.Errorf("expected: %#v\n actual:   %#v", want, got)
	}
}

func testClearLocationOfOrigin(config *clientcmdapi.Config) {
	for key, obj := range config.AuthInfos {
		obj.LocationOfOrigin = ""
		config.AuthInfos[key] = obj
	}
	for key, obj := range config.Clusters {
		obj.LocationOfOrigin = ""
		config.Clusters[key] = obj
	}
	for key, obj := range config.Contexts {
		obj.LocationOfOrigin = ""
		config.Contexts[key] = obj
	}
}

func testSetNilMapsToEmpties(curr reflect.Value) {
	actualCurrValue := curr
	if curr.Kind() == reflect.Ptr {
		actualCurrValue = curr.Elem()
	}

	switch actualCurrValue.Kind() {
	case reflect.Map:
		for _, mapKey := range actualCurrValue.MapKeys() {
			currMapValue := actualCurrValue.MapIndex(mapKey)
			testSetNilMapsToEmpties(currMapValue)
		}

	case reflect.Struct:
		for fieldIndex := 0; fieldIndex < actualCurrValue.NumField(); fieldIndex++ {
			currFieldValue := actualCurrValue.Field(fieldIndex)

			if currFieldValue.Kind() == reflect.Map && currFieldValue.IsNil() {
				newValue := reflect.MakeMap(currFieldValue.Type())
				currFieldValue.Set(newValue)
			} else {
				testSetNilMapsToEmpties(currFieldValue.Addr())
			}
		}

	}

}

func TestExitOption(t *testing.T) {
	gotNeedles := []Needle{
		{"test1", "test2", "any", "*"},
		{"test", "test2", "any", ""},
	}
	u, _ := user.Current()
	wantNeedles := []Needle{
		{"test1", "test2", "any", "*"},
		{"test", "test2", "any", ""},
		{"<Exit>", "exit the kubecm", u.Username, ""},
	}
	type args struct {
		kubeItems []Needle
	}
	tests := []struct {
		name    string
		args    args
		want    []Needle
		wantErr bool
	}{
		// TODO: Add test cases.
		{"test", args{gotNeedles}, wantNeedles, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExitOption(tt.args.kubeItems)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExitOption() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExitOption() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckAndTransformFilePath(t *testing.T) {
	wantPath := homeDir()
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{"test-~", args{path: "~/"}, wantPath, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckAndTransformFilePath(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckAndTransformFilePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CheckAndTransformFilePath() got = %v, want %v", got, tt.want)
			}
		})
	}
}

//func TestCheckValidContext(t *testing.T) {
//	clearWrongConfig := wrongFederalConfig.DeepCopy()
//	clearWrongWant := appendRootConfigConflictAlfa.DeepCopy()
//	type args struct {
//		clear  bool
//		config *clientcmdapi.Config
//	}
//	tests := []struct {
//		name string
//		args args
//		want *clientcmdapi.Config
//	}{
//		// TODO: Add test cases.
//		//{"check-root", args{clear: false, config: &wrongRootConfig}, &appendConfigAlfa},
//		//{"check-federal", args{clear: false, config: &wrongFederalConfig}, &appendRootConfigConflictAlfa},
//		{"clear-federal", args{clear: true, config: clearWrongConfig}, clearWrongWant},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if got := CheckValidContext(tt.args.clear, tt.args.config); !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("CheckValidContext() = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}

func TestCheckValidContext(t *testing.T) {
	tests := []struct {
		name           string
		clear          bool
		config         *clientcmdapi.Config
		expectedConfig *clientcmdapi.Config
	}{
		{
			name:  "Valid Context",
			clear: false,
			config: &clientcmdapi.Config{
				Contexts: map[string]*clientcmdapi.Context{
					"context1": {AuthInfo: "auth1", Cluster: "cluster1"},
				},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{
					"auth1": {},
				},
				Clusters: map[string]*clientcmdapi.Cluster{
					"cluster1": {},
				},
			},
			expectedConfig: &clientcmdapi.Config{
				Contexts: map[string]*clientcmdapi.Context{
					"context1": {AuthInfo: "auth1", Cluster: "cluster1"},
				},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{
					"auth1": {},
				},
				Clusters: map[string]*clientcmdapi.Cluster{
					"cluster1": {},
				},
			},
		},
		//{
		//	name:  "Mismatched AuthInfo and Cluster with Clear=false",
		//	clear: false,
		//	config: &clientcmdapi.Config{
		//		Contexts: map[string]*clientcmdapi.Context{
		//			"context1": {AuthInfo: "auth1", Cluster: "cluster1"},
		//			"context2": {AuthInfo: "auth2", Cluster: "cluster2"},
		//		},
		//		AuthInfos: map[string]*clientcmdapi.AuthInfo{
		//			"auth1": {},
		//		},
		//		Clusters: map[string]*clientcmdapi.Cluster{
		//			"cluster1": {},
		//		},
		//	},
		//	expectedConfig: &clientcmdapi.Config{
		//		Contexts: map[string]*clientcmdapi.Context{
		//			"context1": {AuthInfo: "auth1", Cluster: "cluster1"},
		//		},
		//		AuthInfos: map[string]*clientcmdapi.AuthInfo{
		//			"auth1": {},
		//		},
		//		Clusters: map[string]*clientcmdapi.Cluster{
		//			"cluster1": {},
		//		},
		//	},
		//},
		//{
		//	name:  "Mismatched AuthInfo and Cluster with Clear=true",
		//	clear: true,
		//	config: &clientcmdapi.Config{
		//		Contexts: map[string]*clientcmdapi.Context{
		//			"context1": {AuthInfo: "auth1", Cluster: "cluster1"},
		//			"context2": {AuthInfo: "auth2", Cluster: "cluster2"},
		//		},
		//		AuthInfos: map[string]*clientcmdapi.AuthInfo{
		//			"auth1": {},
		//		},
		//		Clusters: map[string]*clientcmdapi.Cluster{
		//			"cluster1": {},
		//		},
		//	},
		//	expectedConfig: &clientcmdapi.Config{
		//		Contexts: map[string]*clientcmdapi.Context{
		//			"context1": {AuthInfo: "auth1", Cluster: "cluster1"},
		//		},
		//		AuthInfos: map[string]*clientcmdapi.AuthInfo{
		//			"auth1": {},
		//		},
		//		Clusters: map[string]*clientcmdapi.Cluster{
		//			"cluster1": {},
		//		},
		//	},
		//},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckValidContext(tt.clear, tt.config)
			assert.Equal(t, tt.expectedConfig, result)
		})
	}
}

func checkConfig(want, got *clientcmdapi.Config, t *testing.T) {
	if !apiequality.Semantic.DeepEqual(want, got) {
		t.Errorf("diff: %v", diff.ObjectDiff(want, got))
		t.Errorf("expected: %#v\n actual:   %#v", want, got)
	}
}

func Test_getFileName(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "kubecm-get-file-")
	defer os.RemoveAll(tempDir)
	tempFilePath := fmt.Sprintf("%s/%s", tempDir, "testPath")
	_ = ioutil.WriteFile(tempFilePath, []byte{}, 0666)

	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{"TestFileName", args{path: tempFilePath}, "testPath"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFileName(tt.args.path); got != tt.want {
				t.Errorf("getFileName() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testSelectPrompt struct {
	index int
	err   error
}

func (t *testSelectPrompt) Run() (int, string, error) {
	return t.index, "", t.err
}

func TestSelectUI(t *testing.T) {
	kubeItems := []Needle{
		{Name: "Needle1", Cluster: "Cluster1", User: "User1", Center: "Center1"},
		{Name: "Needle2", Cluster: "Cluster2", User: "User2", Center: "Center2"},
		{Name: "<Exit>", Cluster: "", User: "", Center: ""},
	}

	tests := []struct {
		name          string
		selectPrompt  SelectRunner
		expectedIndex int
		expectError   bool
	}{
		{
			name: "Select Needle1",
			selectPrompt: &testSelectPrompt{
				index: 0,
				err:   nil,
			},
			expectedIndex: 0,
			expectError:   false,
		},
		{
			name: "Select <Exit>",
			selectPrompt: &testSelectPrompt{
				index: 2,
				err:   nil,
			},
			expectedIndex: 0,
			expectError:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index, err := selectUIRunner(kubeItems, "Select a needle", test.selectPrompt)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedIndex, index)
			}
		})
	}
}

type testPrompt struct {
	result string
	err    error
}

func (t *testPrompt) Run() (string, error) {
	return t.result, t.err
}

func TestPromptUI(t *testing.T) {
	tests := []struct {
		name        string
		prompt      *testPrompt
		label       string
		defaultName string
		expected    string
		expectErr   bool
	}{
		{
			name: "Valid input",
			prompt: &testPrompt{
				result: "TestName",
				err:    nil,
			},
			label:       "Enter name",
			defaultName: "DefaultName",
			expected:    "TestName",
			expectErr:   false,
		},
		{
			name: "Error occurred",
			prompt: &testPrompt{
				result: "",
				err:    errors.New("Prompt failed"),
			},
			label:       "Enter name",
			defaultName: "DefaultName",
			expected:    "",
			expectErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Capture log output
			logOutput := captureLogOutput(func() {
				_ = promptUIWithRunner(test.prompt)
			})

			if test.expectErr {
				assert.Contains(t, logOutput, "Prompt failed")
			} else {
				assert.Equal(t, test.expected, test.prompt.result)
			}
		})
	}
}

// captureLogOutput captures log output to a string
func captureLogOutput(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	f()
	log.SetOutput(os.Stderr)
	return buf.String()
}

func TestMoreInfo(t *testing.T) {
	// Create a fake client with mock objects
	var clientSet kubernetes.Interface = fake.NewSimpleClientset(
		&corev1.Node{},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}},
	)

	// Create a buffer to capture output from printKV
	buf := bytes.Buffer{}

	// Test the MoreInfo function with the fake client
	err := MoreInfo(clientSet, &buf)
	if err != nil {
		t.Errorf("MoreInfo returned an error: %v", err)
	}

	// Check if the output contains expected values
	expectedOutput := "[Summary] Namespace: 1 Node: 1 Pod: 1 "
	str := strings.Replace(buf.String(), "\n", "", -1)
	if str != expectedOutput {
		t.Errorf("Expected output: %s, got: %s", expectedOutput, buf.String())
	}
}
