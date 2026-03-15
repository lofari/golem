(ns golem.dsl.core-test
  (:require [clojure.test :refer [deftest is testing use-fixtures]]
            [golem.dsl.core :refer [defprimitive defpred]]
            [golem.dsl.registry :as registry]))

(use-fixtures :each (fn [f] (registry/reset-all!) (f)))

(deftest defprimitive-registers-in-registry
  (defprimitive plan
    "Plans from goal."
    {:reads [:goal] :writes [:plan] :session true}
    (fn [state context adapter] {:plan [{:step 1 :desc "do thing"}]}))
  (let [p (registry/get-primitive :plan)]
    (is (some? p))
    (is (= :plan (:name p)))
    (is (= [:goal] (get-in p [:contract :reads])))
    (is (= [:plan] (get-in p [:contract :writes])))))

(deftest defpred-registers-in-registry
  (defpred failed?
    (= :fail (get-in state [:test-results :status])))
  (let [pred-fn (registry/get-predicate :failed?)]
    (is (some? pred-fn))
    (is (true? (pred-fn {:test-results {:status :fail}})))
    (is (false? (pred-fn {:test-results {:status :pass}})))))
