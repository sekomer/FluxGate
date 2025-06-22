package discovery

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/hashicorp/memberlist"
)

type Service struct {
	list       *memberlist.Memberlist
	broadcasts *memberlist.TransmitLimitedQueue
	services   map[string][]ServiceInstance
	mu         sync.RWMutex
	onChange   []func(services map[string][]ServiceInstance)
}

type ServiceInstance struct {
	ID       string            `json:"id"`
	Service  string            `json:"service"`
	Address  string            `json:"address"`
	Port     int               `json:"port"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type broadcast struct {
	msg    []byte
	notify chan<- struct{}
}

func New(port int, joinAddr string) (*Service, error) {
	s := &Service{
		services: make(map[string][]ServiceInstance),
		onChange: make([]func(map[string][]ServiceInstance), 0),
	}

	config := memberlist.DefaultLocalConfig()
	config.BindPort = port
	config.Name = fmt.Sprintf("fluxgate-%d", port)
	config.Delegate = s
	config.Events = s

	list, err := memberlist.Create(config)
	if err != nil {
		return nil, fmt.Errorf("creating memberlist: %w", err)
	}

	s.list = list
	s.broadcasts = &memberlist.TransmitLimitedQueue{
		NumNodes: func() int {
			return list.NumMembers()
		},
		RetransmitMult: 3,
	}

	if joinAddr != "" {
		_, err := list.Join([]string{joinAddr})
		if err != nil {
			return nil, fmt.Errorf("joining cluster: %w", err)
		}
	}

	return s, nil
}

func (s *Service) Register(instance ServiceInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.services[instance.Service] == nil {
		s.services[instance.Service] = make([]ServiceInstance, 0)
	}

	exists := false
	for i, inst := range s.services[instance.Service] {
		if inst.ID == instance.ID {
			s.services[instance.Service][i] = instance
			exists = true
			break
		}
	}

	if !exists {
		s.services[instance.Service] = append(s.services[instance.Service], instance)
	}

	data, err := json.Marshal(map[string]any{
		"action":   "register",
		"instance": instance,
	})
	if err != nil {
		return err
	}

	s.broadcasts.QueueBroadcast(&broadcast{
		msg: data,
	})

	s.notifyListeners()
	return nil
}

func (s *Service) Deregister(serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for service, instances := range s.services {
		for i, inst := range instances {
			if inst.ID == serviceID {
				s.services[service] = append(instances[:i], instances[i+1:]...)

				data, err := json.Marshal(map[string]any{
					"action":     "deregister",
					"service_id": serviceID,
				})
				if err != nil {
					return err
				}

				s.broadcasts.QueueBroadcast(&broadcast{
					msg: data,
				})

				s.notifyListeners()
				return nil
			}
		}
	}

	return fmt.Errorf("service instance not found: %s", serviceID)
}

func (s *Service) GetInstances(service string) []ServiceInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instances := s.services[service]
	if instances == nil {
		return []ServiceInstance{}
	}

	result := make([]ServiceInstance, len(instances))
	copy(result, instances)
	return result
}

func (s *Service) GetAllServices() map[string][]ServiceInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]ServiceInstance)
	for k, v := range s.services {
		result[k] = make([]ServiceInstance, len(v))
		copy(result[k], v)
	}
	return result
}

func (s *Service) Subscribe(fn func(map[string][]ServiceInstance)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = append(s.onChange, fn)
}

func (s *Service) notifyListeners() {
	services := make(map[string][]ServiceInstance)
	for k, v := range s.services {
		services[k] = make([]ServiceInstance, len(v))
		copy(services[k], v)
	}

	for _, fn := range s.onChange {
		go fn(services)
	}
}

func (s *Service) NodeMeta(limit int) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, _ := json.Marshal(s.services)
	if len(data) > limit {
		return nil
	}
	return data
}

func (s *Service) NotifyMsg(msg []byte) {
	var message map[string]any
	if err := json.Unmarshal(msg, &message); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return
	}

	action, ok := message["action"].(string)
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch action {
	case "register":
		if instanceData, ok := message["instance"].(map[string]any); ok {
			var instance ServiceInstance
			data, _ := json.Marshal(instanceData)
			if err := json.Unmarshal(data, &instance); err == nil {
				if s.services[instance.Service] == nil {
					s.services[instance.Service] = make([]ServiceInstance, 0)
				}

				exists := false
				for i, inst := range s.services[instance.Service] {
					if inst.ID == instance.ID {
						s.services[instance.Service][i] = instance
						exists = true
						break
					}
				}

				if !exists {
					s.services[instance.Service] = append(s.services[instance.Service], instance)
				}

				s.notifyListeners()
			}
		}
	case "deregister":
		if serviceID, ok := message["service_id"].(string); ok {
			for service, instances := range s.services {
				for i, inst := range instances {
					if inst.ID == serviceID {
						s.services[service] = append(instances[:i], instances[i+1:]...)
						s.notifyListeners()
						return
					}
				}
			}
		}
	}
}

func (s *Service) GetBroadcasts(overhead, limit int) [][]byte {
	return s.broadcasts.GetBroadcasts(overhead, limit)
}

func (s *Service) LocalState(join bool) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, _ := json.Marshal(s.services)
	return data
}

func (s *Service) MergeRemoteState(buf []byte, join bool) {
	var remoteServices map[string][]ServiceInstance
	if err := json.Unmarshal(buf, &remoteServices); err != nil {
		log.Printf("Failed to unmarshal remote state: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for service, instances := range remoteServices {
		if s.services[service] == nil {
			s.services[service] = instances
		} else {
			for _, remoteInst := range instances {
				exists := false
				for i, localInst := range s.services[service] {
					if localInst.ID == remoteInst.ID {
						s.services[service][i] = remoteInst
						exists = true
						break
					}
				}
				if !exists {
					s.services[service] = append(s.services[service], remoteInst)
				}
			}
		}
	}

	s.notifyListeners()
}

func (s *Service) NotifyJoin(node *memberlist.Node) {
	log.Printf("Node joined: %s", node.Name)
}

func (s *Service) NotifyLeave(node *memberlist.Node) {
	log.Printf("Node left: %s", node.Name)
}

func (s *Service) NotifyUpdate(node *memberlist.Node) {
	log.Printf("Node updated: %s", node.Name)
}

func (b *broadcast) Invalidates(other memberlist.Broadcast) bool {
	return false
}

func (b *broadcast) Message() []byte {
	return b.msg
}

func (b *broadcast) Finished() {
	if b.notify != nil {
		close(b.notify)
	}
}
