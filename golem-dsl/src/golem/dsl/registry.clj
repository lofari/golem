(ns golem.dsl.registry
  "Global graph registry. Stores agent graphs at macroexpand time.")

(defonce ^:private agents (atom {}))
(defonce ^:private primitives (atom {}))
(defonce ^:private predicates (atom {}))

(defn register-agent! [name graph]
  (swap! agents assoc name graph))

(defn register-primitive! [name definition]
  (swap! primitives assoc name definition))

(defn register-predicate! [name pred-fn]
  (swap! predicates assoc name pred-fn))

(defn get-agent [name]
  (get @agents name))

(defn get-primitive [name]
  (get @primitives name))

(defn get-predicate [name]
  (get @predicates name))

(defn all-agents []
  @agents)

(defn all-primitives []
  @primitives)

(defn reset-all! []
  (reset! agents {})
  (reset! primitives {})
  (reset! predicates {}))

;; --- Graph inspection functions ---

(defn nodes
  "Return the nodes list for an agent."
  [agent-name]
  (:nodes (get-agent agent-name)))

(defn edges
  "Return the edges list for an agent."
  [agent-name]
  (:edges (get-agent agent-name)))

(defn contracts
  "Return the contract chain for an agent's nodes."
  [agent-name]
  (mapv :contract (nodes agent-name)))

(defn validate
  "Re-run contract validation on an agent. Returns nil if valid, errors otherwise."
  [agent-name]
  (let [agent (get-agent agent-name)]
    (when agent
      (let [initial-state (get-in agent [:metadata :initial-state] [])]
        (require 'golem.dsl.contracts)
        ((resolve 'golem.dsl.contracts/validate-chain)
         (:nodes agent) initial-state)))))
