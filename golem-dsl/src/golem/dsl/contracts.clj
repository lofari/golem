(ns golem.dsl.contracts
  "Compile-time contract chain validation.")

(defn validate-chain
  "Verify that every step's :reads keys are provided by initial-state
   or a prior step's :writes. Returns nil if valid, vector of errors if not."
  [steps initial-state]
  (loop [remaining steps
         available (set initial-state)
         errors []]
    (if (empty? remaining)
      (when (seq errors) errors)
      (let [{:keys [id contract]} (first remaining)
            reads (:reads contract [])
            writes (:writes contract [])
            missing (remove available reads)]
        (recur (rest remaining)
               (into available writes)
               (into errors
                     (map (fn [k] {:node id :missing-key k :available available})
                          missing)))))))
