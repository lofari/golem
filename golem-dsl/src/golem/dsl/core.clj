(ns golem.dsl.core
  "DSL macros: defagent, defprimitive, defpred."
  (:require [golem.dsl.registry :as registry]
            [golem.dsl.contracts :as contracts]))

(defmacro defprimitive
  "Define a primitive with inline contract.
   (defprimitive plan
     \"Plans from goal.\"
     {:reads [:goal] :writes [:plan] :session true}
     (fn [state context adapter] ...))"
  [name docstring contract body]
  `(registry/register-primitive!
    ~(keyword name)
    {:name ~(keyword name)
     :doc ~docstring
     :contract ~contract
     :execute ~body}))

(defmacro defpred
  "Define a predicate over state.
   (defpred failed? (= :fail (get-in state [:test-results :status])))"
  [name body]
  `(registry/register-predicate!
    ~(keyword name)
    (fn [~'state] ~body)))

(defn- parse-body
  "Separate metadata, steps, and error handlers from defagent body.
   Supports optional docstring before metadata map."
  [body]
  (let [;; Skip optional docstring
        body (if (string? (first body)) (rest body) body)
        metadata (when (map? (first body)) (first body))
        rest-body (if metadata (rest body) body)
        {error-handlers true steps false}
        (group-by #(and (seq? %) (= 'on-error (first %))) rest-body)]
    {:metadata (or metadata {})
     :steps (vec steps)
     :error-handlers (vec error-handlers)}))

(defn- resolve-step
  "Resolve a step form to a node map. Returns {:id :primitive :contract}."
  [step-form counter]
  (let [primitive-name (if (seq? step-form) (first step-form) step-form)
        prim-key (keyword primitive-name)
        prim (registry/get-primitive prim-key)
        _ (when-not prim
            (throw (ex-info (str "Unknown primitive: " primitive-name)
                            {:primitive primitive-name})))
        id-num (get (swap! counter update prim-key (fnil inc 0)) prim-key)
        node-id (keyword (str (name prim-key) "-" id-num))]
    {:id node-id
     :primitive prim-key
     :contract (:contract prim)}))

(defn- control-flow?
  "Check if a step form is a control flow expression."
  [form]
  (and (seq? form) (#{'while 'if 'when} (first form))))

(defn- expand-steps-inner
  "Expand step forms into {:nodes [...] :edges [...]}. Handles control flow."
  [steps counter]
  (reduce
   (fn [{:keys [nodes edges]} step]
     (if (control-flow? step)
       (let [op (first step)]
         (case op
           ;; (while pred? {:max N} body...)
           while
           (let [pred-name (second step)
                 opts (if (map? (nth step 2)) (nth step 2) {})
                 body-forms (if (map? (nth step 2))
                              (drop 3 step)
                              (drop 2 step))
                 body-result (expand-steps-inner (vec body-forms) counter)
                 body-nodes (:nodes body-result)
                 body-edges (:edges body-result)
                 prev-node (last nodes)
                 first-body (first body-nodes)
                 last-body (last body-nodes)]
             {:nodes (into nodes body-nodes)
              :edges (-> edges
                         ;; Sequential edges within loop body
                         (into body-edges)
                         ;; Edge from prev node into loop body (conditional: when pred)
                         (conj {:from (:id prev-node)
                                :to (:id first-body)
                                :when (keyword pred-name)
                                :max (:max opts)})
                         ;; Loop-back edge from last body to first body
                         (conj {:from (:id last-body)
                                :to (:id first-body)
                                :when (keyword pred-name)
                                :max (:max opts)})
                         ;; Forward edge when pred is false (skip loop)
                         (conj {:from (:id prev-node)
                                :to :next
                                :when-not (keyword pred-name)}))})

           ;; (if pred? then-step else-step)
           if
           (let [pred-name (second step)
                 then-form (nth step 2)
                 else-form (nth step 3)
                 then-node (resolve-step then-form counter)
                 else-node (resolve-step else-form counter)
                 prev-node (last nodes)]
             {:nodes (-> nodes (conj then-node) (conj else-node))
              :edges (-> edges
                         (conj {:from (:id prev-node)
                                :to (:id then-node)
                                :when (keyword pred-name)})
                         (conj {:from (:id prev-node)
                                :to (:id else-node)
                                :when-not (keyword pred-name)}))})

           ;; (when pred? body...)
           when
           (let [pred-name (second step)
                 body-forms (drop 2 step)
                 body-result (expand-steps-inner (vec body-forms) counter)
                 body-nodes (:nodes body-result)
                 body-edges (:edges body-result)
                 prev-node (last nodes)
                 first-body (first body-nodes)]
             {:nodes (into nodes body-nodes)
              :edges (-> edges
                         (into body-edges)
                         (conj {:from (:id prev-node)
                                :to (:id first-body)
                                :when (keyword pred-name)})
                         (conj {:from (:id prev-node)
                                :to :next
                                :when-not (keyword pred-name)}))})))

       ;; Regular primitive step
       (let [node (resolve-step step counter)
             prev-node (last nodes)]
         {:nodes (conj nodes node)
          :edges (if prev-node
                   (conj edges [(:id prev-node) (:id node)])
                   edges)})))
   {:nodes [] :edges []}
   steps))

(defn expand-steps
  "Expand step forms into flat node list, handling control flow."
  [steps]
  (:nodes (expand-steps-inner steps (atom {}))))

(defn build-edges
  "Build edges from step forms. Returns mix of vectors and maps for conditional edges."
  [steps]
  (:edges (expand-steps-inner steps (atom {}))))

(defmacro defagent
  "Define an agent program.
   (defagent name
     {:initial-state [:goal]}
     (plan)
     (implement)
     (review)
     (on-error :transient (retry {:max 3})))"
  [name & body]
  (let [{:keys [metadata steps error-handlers]} (parse-body body)
        initial-state (:initial-state metadata [])]
    ;; Contract validation and graph registration happen at runtime
    ;; (after primitives are registered), not at macroexpand time
    `(let [nodes# (expand-steps '~steps)
           errors# (contracts/validate-chain nodes# ~initial-state)]
       (when errors#
         (throw (ex-info (str "Contract validation failed for agent " '~name)
                         {:agent '~name :errors errors#})))
       (registry/register-agent!
        ~(keyword name)
        {:name ~(keyword name)
         :metadata ~metadata
         :nodes nodes#
         :edges (build-edges '~steps)
         :error-handlers '~error-handlers}))))
