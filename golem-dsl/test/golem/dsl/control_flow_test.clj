(ns golem.dsl.control-flow-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.core :refer [defagent defprimitive defpred]]
            [golem.dsl.registry :as registry]))

(use-fixtures :each (fn [f] (registry/reset-all!) (f)))

(defn- setup! []
  (defprimitive plan
    "Plans." {:reads [:goal] :writes [:plan] :session true}
    (fn [s c a] {:plan []}))
  (defprimitive implement
    "Implements." {:reads [:goal :plan] :optional-reads [:review-feedback]
                   :writes [:code :test-results] :session true}
    (fn [s c a] {}))
  (defprimitive review
    "Reviews." {:reads [:code :test-results] :writes [:review-feedback] :session true}
    (fn [s c a] {}))
  (defpred needs-work?
    (= :needs-work (get-in state [:review-feedback :verdict]))))

(deftest while-loop-creates-conditional-edges
  (setup!)
  (defagent with-loop
    {:initial-state [:goal]}
    (plan)
    (implement)
    (review)
    (while needs-work? {:max 3}
      (implement)
      (review)))
  (let [agent (registry/get-agent :with-loop)]
    (is (some? agent))
    ;; Should have nodes from both the linear steps and the loop body
    (is (= 5 (count (:nodes agent))))
    ;; Should have conditional edges from the while predicate
    (is (some #(map? %) (:edges agent)))))

(deftest if-creates-conditional-edges
  (setup!)
  (defagent with-if
    {:initial-state [:goal]}
    (plan)
    (implement)
    (review)
    (if needs-work?
      (implement)
      (plan)))
  (let [agent (registry/get-agent :with-if)]
    (is (some? agent))
    ;; plan + implement + review + if-then implement + if-else plan = 5
    (is (= 5 (count (:nodes agent))))
    (is (some #(map? %) (:edges agent)))))

(deftest when-creates-conditional-edges
  (setup!)
  (defagent with-when
    {:initial-state [:goal]}
    (plan)
    (implement)
    (review)
    (when needs-work?
      (implement)))
  (let [agent (registry/get-agent :with-when)]
    (is (some? agent))
    (is (= 4 (count (:nodes agent))))
    (is (some #(map? %) (:edges agent)))))
