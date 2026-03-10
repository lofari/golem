(ns golem.dsl.engine.core
  "Execution engine: walks agent graph, manages state, handles errors."
  (:require [golem.dsl.registry :as registry]
            [golem.dsl.engine.state :as state]
            [golem.dsl.engine.snapshot :as snapshot]
            [golem.dsl.engine.context :as context]
            [golem.dsl.engine.output :as output]
            [golem.dsl.errors.types :as error-types]
            [golem.dsl.errors.handler :as error-handler]
            [golem.dsl.errors.diagnostic :as diagnostic]
            [golem.dsl.session.protocol :as proto]))

(defn- execute-session-primitive
  "Execute a primitive that requires a Claude session."
  [node current-state adapter working-dir]
  (let [prim (registry/get-primitive (:primitive node))
        contract (:contract prim)
        prompt (context/render-prompt (:primitive node) current-state contract)
        before-files (output/scan-files working-dir)
        handle (proto/spawn adapter prompt working-dir {})
        wait-result (proto/await-result adapter handle 600000)]
    (if (= 0 (:exit-code wait-result))
      (let [session-output (proto/read-output adapter handle)
            collected (output/collect-output working-dir before-files contract)
            merged (merge session-output collected)]
        {:ok true :output merged})
      {:ok false
       :error {:exit-code (:exit-code wait-result)
               :timeout (:timeout wait-result)}})))

(defn- execute-local-primitive
  "Execute a primitive that runs locally (no session)."
  [node current-state]
  (let [prim (registry/get-primitive (:primitive node))
        execute-fn (:execute prim)
        result (execute-fn current-state {} nil)]
    {:ok true :output result}))

(defn- execute-node
  "Execute a single node. Returns {:ok bool :output map} or {:ok false :error map}."
  [node current-state adapter working-dir]
  (let [prim (registry/get-primitive (:primitive node))
        contract (:contract prim)]
    ;; Validate reads
    (if-let [read-error (state/validate-reads current-state (:reads contract []))]
      {:ok false :error (assoc read-error :type :contract-violation)}
      ;; Execute based on session type
      (if (:session contract)
        (execute-session-primitive node current-state adapter working-dir)
        (execute-local-primitive node current-state)))))

(defn- find-next-node
  "Given current node index and edges, find the next node to execute.
   Returns index into nodes vector, or nil if done."
  [current-idx nodes edges current-state]
  (let [current-node (nth nodes current-idx)
        current-id (:id current-node)
        ;; Find edges from current node
        matching-edges (filter (fn [e]
                                 (if (map? e)
                                   (= current-id (:from e))
                                   (= current-id (first e))))
                               edges)]
    (if (empty? matching-edges)
      ;; No explicit edges — try sequential (next index)
      (let [next-idx (inc current-idx)]
        (when (< next-idx (count nodes))
          next-idx))
      ;; Evaluate edges
      (let [chosen (some (fn [e]
                           (if (map? e)
                             ;; Conditional edge
                             (let [pred-key (or (:when e) (:when-not e))
                                   pred-fn (registry/get-predicate pred-key)
                                   pred-result (if pred-fn (pred-fn current-state) false)
                                   matches? (if (:when e) pred-result (not pred-result))]
                               (when matches?
                                 (:to e)))
                             ;; Simple edge [from to]
                             (second e)))
                         matching-edges)]
        (when (and chosen (not= chosen :next))
          ;; Find index of target node
          (some (fn [i] (when (= chosen (:id (nth nodes i))) i))
                (range (count nodes))))))))

(defn- handle-error
  "Handle an error from node execution. Returns {:action :retry|:halt, ...}."
  [error node agent-graph opts attempt]
  (let [error-type (error-types/classify error)
        agent-handlers (some-> (:error-handlers agent-graph)
                               (->> (map (fn [h] [(second h) (last h)]))
                                    (into {})))
        handler (error-handler/resolve-handler error-type nil agent-handlers)]
    (if (error-handler/should-retry? handler attempt)
      {:action :retry :handler handler}
      {:action :halt :handler handler :error-type error-type})))

(defn run
  "Execute an agent program. Returns {:state map :version int :run-id keyword :status keyword}."
  [agent-name initial-state opts]
  (let [agent-graph (registry/get-agent agent-name)
        _ (when-not agent-graph
            (throw (ex-info (str "Unknown agent: " agent-name)
                            {:agent agent-name})))
        {:keys [adapter base-dir]} opts
        working-dir (or (:working-dir opts) base-dir ".")
        run-id (snapshot/next-run-id base-dir)
        nodes (:nodes agent-graph)
        edges (:edges agent-graph)]

    ;; Save initial state and graph
    (snapshot/save-state! base-dir run-id 0 initial-state)
    (snapshot/save-graph! base-dir run-id agent-graph)

    ;; Walk the graph
    (loop [node-idx 0
           current-state initial-state
           version 0
           attempts {}]
      (if (or (nil? node-idx) (>= node-idx (count nodes)))
        ;; Done
        {:state current-state
         :version version
         :run-id run-id
         :status :completed}

        (let [node (nth nodes node-idx)
              result (execute-node node current-state adapter working-dir)]
          (if (:ok result)
            ;; Success: apply writes, snapshot, advance
            (let [prim (registry/get-primitive (:primitive node))
                  contract (:contract prim)
                  new-state (state/apply-writes current-state
                                                (:output result)
                                                (:writes contract []))
                  new-version (inc version)]
              (snapshot/save-state! base-dir run-id new-version new-state)
              (snapshot/save-log! base-dir run-id
                                 {:node (:id node)
                                  :primitive (:primitive node)
                                  :version new-version
                                  :status :ok})
              (let [next-idx (find-next-node node-idx nodes edges new-state)]
                (recur next-idx new-state new-version {})))

            ;; Error: classify, handle
            (let [node-key (:id node)
                  attempt (get attempts node-key 0)
                  decision (handle-error (:error result) node agent-graph opts attempt)]
              (diagnostic/save-diagnostic! base-dir run-id version
                                          {:node (:id node)
                                           :error (:error result)
                                           :attempt attempt
                                           :decision decision})
              (if (= :retry (:action decision))
                (recur node-idx current-state version
                       (update attempts node-key (fnil inc 0)))
                ;; Halt
                (do
                  (snapshot/save-state! base-dir run-id (inc version) current-state)
                  {:state current-state
                   :version (inc version)
                   :run-id run-id
                   :status :halted
                   :error (:error result)})))))))))
