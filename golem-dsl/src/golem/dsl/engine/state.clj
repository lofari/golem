(ns golem.dsl.engine.state
  "Immutable state management with contract enforcement.")

(defn apply-writes
  "Merge output into state, enforcing that only declared keys are written."
  [state output allowed-keys]
  (let [allowed (set allowed-keys)
        extra (remove allowed (keys output))]
    (when (seq extra)
      (throw (ex-info (str "Undeclared write keys: " (vec extra))
                      {:extra-keys (vec extra)
                       :allowed-keys allowed-keys})))
    (merge state (select-keys output allowed-keys))))

(defn validate-reads
  "Check that all required read keys exist in state."
  [state reads]
  (let [missing (remove #(contains? state %) reads)]
    (when (seq missing)
      {:type :contract-violation
       :missing-keys (vec missing)
       :available-keys (vec (keys state))})))

(defn select-reads
  "Extract only the reads and optional-reads keys from state."
  [state reads optional-reads]
  (merge (select-keys state reads)
         (select-keys state (or optional-reads []))))
